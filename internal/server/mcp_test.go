package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/mail"
)

// newMCPTestServer 建立測試 Server 並發一把 API Key。
func newMCPTestServer(t *testing.T, f *fakeBackend) (*httptest.Server, string) {
	t.Helper()
	s, ts := newTestServer(f)
	t.Cleanup(ts.Close)
	_, token, err := s.keys.Create("test")
	if err != nil {
		t.Fatal(err)
	}
	return ts, token
}

// mcpPost 送出一則 JSON-RPC 訊息,返回狀態碼、原始回應與標頭。
func mcpPost(t *testing.T, ts *httptest.Server, token, body string, headers map[string]string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/api/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw), resp.Header
}

type mcpTestResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func decodeMCP(t *testing.T, body string) mcpTestResp {
	t.Helper()
	var out mcpTestResp
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("回應不是 JSON: %v: %s", err, body)
	}
	if out.JSONRPC != "2.0" {
		t.Fatalf("回應缺少 jsonrpc 2.0: %s", body)
	}
	return out
}

type mcpToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent map[string]any `json:"structuredContent"`
	IsError           bool           `json:"isError"`
}

func callTool(t *testing.T, ts *httptest.Server, token, name, args string) mcpToolResult {
	t.Helper()
	status, body, _ := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`, nil)
	if status != http.StatusOK {
		t.Fatalf("tools/call 期望 200,得到 %d: %s", status, body)
	}
	resp := decodeMCP(t, body)
	if resp.Error != nil {
		t.Fatalf("tools/call 不應回協定錯誤: %s", body)
	}
	var out mcpToolResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" {
		t.Fatalf("工具結果應有一段 text content: %s", body)
	}
	return out
}

func TestMCPRequiresAPIKey(t *testing.T) {
	ts, _ := newMCPTestServer(t, &fakeBackend{})
	for _, token := range []string{"", "hme_wrong"} {
		status, body, header := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("token=%q 期望 401,得到 %d", token, status)
		}
		if !strings.HasPrefix(header.Get("WWW-Authenticate"), "Bearer") {
			t.Fatalf("401 應帶 WWW-Authenticate: Bearer,得到 %q", header.Get("WWW-Authenticate"))
		}
		if decodeMCP(t, body).Error == nil {
			t.Fatalf("401 應回 JSON-RPC 錯誤: %s", body)
		}
	}
}

// TestMCPHandshake 走一遍標準用戶端流程:initialize → notifications/initialized → ping。
func TestMCPHandshake(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})

	status, body, header := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":"init-1","method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`, nil)
	if status != http.StatusOK || !strings.HasPrefix(header.Get("Content-Type"), "application/json") {
		t.Fatalf("initialize 期望 200 application/json,得到 %d %q: %s", status, header.Get("Content-Type"), body)
	}
	resp := decodeMCP(t, body)
	if string(resp.ID) != `"init-1"` {
		t.Fatalf("id 應原樣回傳,得到 %s", resp.ID)
	}
	var init struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &init); err != nil {
		t.Fatal(err)
	}
	if init.ProtocolVersion != "2025-06-18" {
		t.Fatalf("支援的版本應原樣協商,得到 %q", init.ProtocolVersion)
	}
	if _, ok := init.Capabilities["tools"]; !ok {
		t.Fatalf("capabilities 應宣告 tools: %s", body)
	}
	if init.ServerInfo.Name == "" || init.ServerInfo.Version == "" {
		t.Fatalf("serverInfo 應有 name/version: %s", body)
	}
	if header.Get("Mcp-Session-Id") != "" {
		t.Fatal("無狀態伺服器不應發 Mcp-Session-Id")
	}

	status, body, _ = mcpPost(t, ts, token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, map[string]string{"MCP-Protocol-Version": "2025-06-18"})
	if status != http.StatusAccepted || body != "" {
		t.Fatalf("通知期望 202 且無內容,得到 %d: %q", status, body)
	}

	status, body, _ = mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":2,"method":"ping"}`, map[string]string{"MCP-Protocol-Version": "2025-06-18"})
	if status != http.StatusOK || string(decodeMCP(t, body).Result) != "{}" {
		t.Fatalf("ping 期望空物件結果,得到 %d: %s", status, body)
	}
}

func TestMCPInitializeUnknownVersionFallsBackToLatest(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})
	_, body, _ := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2099-01-01"}}`, nil)
	if !strings.Contains(string(decodeMCP(t, body).Result), `"protocolVersion":"`+mcpProtocolVersions[0]+`"`) {
		t.Fatalf("不支援的版本應回最新版: %s", body)
	}
}

// TestMCPRejectsUnsupportedVersionHeader 驗證 2026-07-28 等版本拿到 400,
// 且不是 -32022:否則雙版本用戶端會認定這是新版伺服器而不退回 initialize。
func TestMCPRejectsUnsupportedVersionHeader(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})
	status, body, _ := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":1,"method":"server/discover"}`, map[string]string{"MCP-Protocol-Version": "2026-07-28"})
	if status != http.StatusBadRequest {
		t.Fatalf("期望 400,得到 %d: %s", status, body)
	}
	if e := decodeMCP(t, body).Error; e == nil || e.Code == -32022 {
		t.Fatalf("應回非 -32022 的 JSON-RPC 錯誤: %s", body)
	}
}

func TestMCPToolsListHasSchemas(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})
	_, body, _ := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	var out struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string         `json:"type"`
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(decodeMCP(t, body).Result, &out); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range out.Tools {
		names[tool.Name] = true
		if tool.Description == "" || tool.InputSchema.Type != "object" || tool.InputSchema.Properties == nil {
			t.Fatalf("工具 %s 缺少 description 或 inputSchema: %s", tool.Name, body)
		}
		if tool.Name != "list_accounts" && (len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "account_id") {
			t.Fatalf("工具 %s 應要求 account_id: %+v", tool.Name, tool.InputSchema.Required)
		}
	}
	for _, want := range []string{"list_accounts", "create_alias", "list_inbox", "get_verification_code"} {
		if !names[want] {
			t.Fatalf("tools/list 缺少 %s: %s", want, body)
		}
	}
}

func TestMCPCallListAccounts(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{accounts: []account.Summary{{ID: "acc_1", Name: "主帳號"}}})
	res := callTool(t, ts, token, "list_accounts", `{}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, `"acc_1"`) {
		t.Fatalf("list_accounts 結果錯誤: %+v", res)
	}
	accounts, _ := res.StructuredContent["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("structuredContent.accounts 應有 1 筆: %+v", res.StructuredContent)
	}
}

func TestMCPCallCreateAlias(t *testing.T) {
	f := &fakeBackend{created: &hme.CreateResult{Email: "new@icloud.com", Label: "shop", CreatedAt: "2026-10-30T00:00:00Z"}}
	ts, token := newMCPTestServer(t, f)
	res := callTool(t, ts, token, "create_alias", `{"account_id":"acc_1","label":"shop"}`)
	if res.IsError || res.StructuredContent["email"] != "new@icloud.com" {
		t.Fatalf("create_alias 結果錯誤: %+v", res)
	}
	if f.createdAccountID != "acc_1" || f.createdLabel != "shop" {
		t.Fatalf("參數未傳到 backend: %q %q", f.createdAccountID, f.createdLabel)
	}
}

// TestMCPToolErrorsAreResults 驗證參數錯誤與後端失敗以 isError 結果回傳,讓模型能看到原因。
func TestMCPToolErrorsAreResults(t *testing.T) {
	f := &fakeBackend{createdErr: &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}}
	ts, token := newMCPTestServer(t, f)
	cases := []struct{ tool, args, want string }{
		{"create_alias", `{}`, "account_id"},
		{"create_alias", `null`, "account_id"},
		{"create_alias", `{"account_id":"acc_x"}`, "ACCOUNT_NOT_FOUND"},
		{"list_inbox", `{"account_id":"acc_1","limit":500}`, "limit"},
		{"list_inbox", `{"account_id":"acc_1","limit":"20"}`, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		res := callTool(t, ts, token, tc.tool, tc.args)
		if !res.IsError || !strings.Contains(res.Content[0].Text, tc.want) {
			t.Fatalf("%s %s 應回 isError 並提到 %q: %+v", tc.tool, tc.args, tc.want, res)
		}
	}
}

func TestMCPCallListInboxDefaults(t *testing.T) {
	f := &fakeBackend{inbox: InboxResult{AccountID: "acc_1", Messages: []mail.Message{{ID: "1", Subject: "hi"}}}}
	ts, token := newMCPTestServer(t, f)
	res := callTool(t, ts, token, "list_inbox", `{"account_id":"acc_1","alias":" a@icloud.com "}`)
	if res.IsError {
		t.Fatalf("list_inbox 不應失敗: %+v", res)
	}
	want := InboxQuery{AccountID: "acc_1", Alias: "a@icloud.com", Limit: 20, Days: 7}
	if f.listInboxQuery != want {
		t.Fatalf("查詢參數錯誤:期望 %+v,得到 %+v", want, f.listInboxQuery)
	}
}

func TestMCPCallGetVerificationCode(t *testing.T) {
	f := &fakeBackend{inbox: InboxResult{AccountID: "acc_1", Messages: []mail.Message{
		{ID: "1", Subject: "舊的", Date: "2026-10-30T08:00:00+08:00", VerificationCode: "111111"},
		{ID: "2", Subject: "沒有碼", Date: "2026-10-30T10:00:00+08:00"},
		{ID: "3", Subject: "新的", Date: "2026-10-30T09:00:00+08:00", VerificationCode: "333333"},
	}}}
	ts, token := newMCPTestServer(t, f)
	res := callTool(t, ts, token, "get_verification_code", `{"account_id":"acc_1","alias":"a@icloud.com"}`)
	if res.IsError || res.StructuredContent["found"] != true || res.StructuredContent["code"] != "333333" {
		t.Fatalf("應挑出最新的驗證碼 333333: %+v", res.StructuredContent)
	}
	if f.listInboxQuery.Days != 1 || f.listInboxQuery.Alias != "a@icloud.com" {
		t.Fatalf("查詢參數錯誤: %+v", f.listInboxQuery)
	}

	f.inbox.Messages = []mail.Message{{ID: "4", Subject: "沒有碼"}}
	res = callTool(t, ts, token, "get_verification_code", `{"account_id":"acc_1"}`)
	if res.IsError || res.StructuredContent["found"] != false {
		t.Fatalf("沒有驗證碼時應回 found=false: %+v", res.StructuredContent)
	}
}

func TestMCPProtocolErrors(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})
	cases := []struct {
		name, body string
		status     int
		code       int
	}{
		{"未知方法", `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, http.StatusOK, rpcMethodNotFound},
		{"未知工具", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope"}}`, http.StatusOK, rpcInvalidParams},
		{"缺少工具名稱", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, http.StatusOK, rpcInvalidParams},
		{"JSON 錯誤", `{"jsonrpc":`, http.StatusBadRequest, rpcParseError},
		{"批次", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, http.StatusBadRequest, rpcInvalidRequest},
		{"缺少 jsonrpc", `{"id":1,"method":"ping"}`, http.StatusBadRequest, rpcInvalidRequest},
	}
	for _, tc := range cases {
		status, body, _ := mcpPost(t, ts, token, tc.body, nil)
		resp := decodeMCP(t, body)
		if status != tc.status || resp.Error == nil || resp.Error.Code != tc.code {
			t.Fatalf("%s: 期望 %d / %d,得到 %d: %s", tc.name, tc.status, tc.code, status, body)
		}
	}
}

func TestMCPClientResponseAccepted(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})
	status, body, _ := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":5,"result":{}}`, nil)
	if status != http.StatusAccepted || body != "" {
		t.Fatalf("用戶端回應期望 202 且無內容,得到 %d: %q", status, body)
	}
}

func TestMCPRejectsCrossOrigin(t *testing.T) {
	ts, token := newMCPTestServer(t, &fakeBackend{})
	status, _, _ := mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": "https://evil.example"})
	if status != http.StatusForbidden {
		t.Fatalf("跨來源 Origin 期望 403,得到 %d", status)
	}
	status, _, _ = mcpPost(t, ts, token, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": ts.URL})
	if status != http.StatusOK {
		t.Fatalf("同源 Origin 期望 200,得到 %d", status)
	}
}

func TestMCPGetAndDeleteNotAllowed(t *testing.T) {
	ts, _ := newMCPTestServer(t, &fakeBackend{})
	for _, method := range []string{"GET", "DELETE"} {
		req, _ := http.NewRequest(method, ts.URL+"/api/mcp", nil)
		req.Header.Set("Accept", "text/event-stream")
		status, _, _ := do(t, req)
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("%s 期望 405,得到 %d", method, status)
		}
	}
}
