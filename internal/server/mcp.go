// Package server - MCP(Model Context Protocol)端點。
//
// 實作 Streamable HTTP 傳輸的無狀態子集:
//
//	POST   /api/mcp  — 每個 POST 一則 JSON-RPC 2.0 訊息;請求一律回 application/json,
//	                   通知(沒有 id)回 202 且無內容
//	GET    /api/mcp  — 405,不提供 SSE 串流
//	DELETE /api/mcp  — 405,不發 Mcp-Session-Id,也就沒有工作階段可結束
//
// 支援以 initialize 握手的協定版本(見 mcpProtocolVersions)。2026-07-28 起的
// 「每個請求帶 _meta」版本會收到不帶 -32022 的 400,依規範雙版本用戶端會改走 initialize。
//
// 認證走 API Key(Authorization: Bearer),與管理員工作階段無關。這裡的 401 只有
// MCP 用戶端會看到,前端不會呼叫此端點,因此不違反「401 = 管理員工作階段失效」的約定。
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/account"
	"icloud-hme/internal/mail"
)

// mcpProtocolVersions 是支援的 MCP 協定版本,第一個是最新版。
var mcpProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpServerVersion = "1.0.0"

const mcpInstructions = "Manage iCloud Hide My Email aliases and read the mail they receive. " +
	"Typical flow: call list_accounts to get an account_id, call create_alias to get a fresh email address, " +
	"use that address to sign up somewhere, then call get_verification_code with the same account_id and alias. " +
	"If found is false, the email has probably not arrived yet: wait a few seconds and call it again."

// JSON-RPC 2.0 標準錯誤碼。
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

// rpcMessage 是收到的 JSON-RPC 訊息;沒有 id 為通知,沒有 method 但有 result/error 為用戶端回應。
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// writeRPC 寫出 JSON-RPC 回應;無法得知請求 id 時依規範回 null。
func writeRPC(c *gin.Context, status int, id json.RawMessage, result any, rerr *rpcError) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	c.AbortWithStatusJSON(status, rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: rerr})
}

func (s *Server) handleMCP(c *gin.Context) {
	if !mcpOriginAllowed(c.Request) {
		writeRPC(c, http.StatusForbidden, nil, nil, &rpcError{Code: rpcInvalidRequest, Message: "Origin 不被允許"})
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	if _, valid := s.keys.Authenticate(token); !valid {
		c.Header("WWW-Authenticate", `Bearer realm="icloud-hme"`)
		writeRPC(c, http.StatusUnauthorized, nil, nil, &rpcError{Code: rpcInvalidRequest, Message: "需要有效的 API Key：Authorization: Bearer <API_KEY>"})
		return
	}

	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes))
	if err != nil || !json.Valid(raw) {
		writeRPC(c, http.StatusBadRequest, nil, nil, &rpcError{Code: rpcParseError, Message: "JSON 解析失敗"})
		return
	}
	var msg rpcMessage
	// 陣列(批次)在 2025-06-18 已從規範移除,與其他非物件內容一起當作無效請求。
	if err := json.Unmarshal(raw, &msg); err != nil || msg.JSONRPC != "2.0" {
		writeRPC(c, http.StatusBadRequest, msg.ID, nil, &rpcError{Code: rpcInvalidRequest, Message: "不是有效的 JSON-RPC 2.0 訊息"})
		return
	}
	if msg.Method == "" {
		if len(msg.ID) > 0 && (msg.Result != nil || msg.Error != nil) {
			c.Status(http.StatusAccepted)
			return
		}
		writeRPC(c, http.StatusBadRequest, msg.ID, nil, &rpcError{Code: rpcInvalidRequest, Message: "缺少 method"})
		return
	}
	// initialize 在 body 內協商版本;其餘請求依標頭判斷,未帶標頭時規範要求假設為相容版本。
	if v := c.GetHeader("MCP-Protocol-Version"); v != "" && msg.Method != "initialize" && !slices.Contains(mcpProtocolVersions, v) {
		writeRPC(c, http.StatusBadRequest, msg.ID, nil, &rpcError{Code: rpcInvalidRequest, Message: fmt.Sprintf("不支援的 MCP 協定版本 %q，支援：%s", v, strings.Join(mcpProtocolVersions, ", "))})
		return
	}
	if len(msg.ID) == 0 {
		// notifications/initialized、notifications/cancelled 等都不需要處理。
		c.Status(http.StatusAccepted)
		return
	}

	result, rerr := s.dispatchMCP(msg.Method, msg.Params)
	writeRPC(c, http.StatusOK, msg.ID, result, rerr)
}

func (s *Server) dispatchMCP(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		version := mcpProtocolVersions[0]
		if slices.Contains(mcpProtocolVersions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		return gin.H{
			"protocolVersion": version,
			"capabilities":    gin.H{"tools": gin.H{}},
			"serverInfo":      gin.H{"name": "icloud-hme", "title": "iCloud HME", "version": mcpServerVersion},
			"instructions":    mcpInstructions,
		}, nil
	case "ping":
		return gin.H{}, nil
	case "tools/list":
		return gin.H{"tools": mcpTools}, nil
	case "tools/call":
		return s.callMCPTool(params)
	default:
		return nil, &rpcError{Code: rpcMethodNotFound, Message: "Method not found: " + method}
	}
}

// callMCPTool 執行工具。工具本身的失敗(參數錯、帳號不存在、上游失敗)以 isError
// 結果回傳,讓模型看得到原因並自行修正;只有工具不存在才是協定錯誤。
func (s *Server) callMCPTool(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "tools/call 需要 params.name"}
	}
	i := slices.IndexFunc(mcpTools, func(t mcpTool) bool { return t.Name == p.Name })
	if i < 0 {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "Unknown tool: " + p.Name}
	}
	data, err := mcpTools[i].call(s, p.Arguments)
	if err != nil {
		be := asBackendError(err)
		return gin.H{"content": []gin.H{{"type": "text", "text": be.Code + ": " + be.Message}}, "isError": true}, nil
	}
	text, err := json.Marshal(data)
	if err != nil {
		return gin.H{"content": []gin.H{{"type": "text", "text": "INTERNAL_ERROR: 內部錯誤"}}, "isError": true}, nil
	}
	// 2025-06-18 起的用戶端讀 structuredContent;依規範同時附上序列化的文字版本給舊用戶端。
	return gin.H{"content": []gin.H{{"type": "text", "text": string(text)}}, "structuredContent": data}, nil
}

func mcpMethodNotAllowed(c *gin.Context) {
	c.Header("Allow", http.MethodPost)
	c.AbortWithStatus(http.StatusMethodNotAllowed)
}

// mcpOriginAllowed 依規範驗證 Origin 以防 DNS rebinding:非瀏覽器的 MCP 用戶端
// 不會帶 Origin;帶了就必須與請求的 Host 同源。
func mcpOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

// ====================================================================
// 工具
// ====================================================================

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`
	call        func(*Server, json.RawMessage) (any, error)
}

func mcpObjectSchema(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

var (
	mcpAccountIDProp = map[string]any{"type": "string", "description": "Account id (e.g. acc_xxx) from list_accounts."}
	mcpAliasProp     = map[string]any{"type": "string", "description": "Optional. Only include emails sent to this Hide My Email address (e.g. the email returned by create_alias)."}
)

var mcpTools = []mcpTool{
	{
		Name:        "list_accounts",
		Title:       "List iCloud accounts",
		Description: "List the iCloud accounts configured on this server. Each account's id is the account_id required by the other tools. Also returns name, iCloud email, status and alias counts.",
		InputSchema: mcpObjectSchema(map[string]any{}),
		Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": false},
		call:        (*Server).mcpListAccounts,
	},
	{
		Name:        "create_alias",
		Title:       "Create Hide My Email alias",
		Description: "Create a new iCloud Hide My Email address under an account and return it. Mail sent to the address is forwarded to the account's inbox and can be read with list_inbox or get_verification_code.",
		InputSchema: mcpObjectSchema(map[string]any{
			"account_id": mcpAccountIDProp,
			"label":      map[string]any{"type": "string", "maxLength": 200, "description": "Optional label shown in iCloud settings, e.g. the site you are signing up for."},
		}, "account_id"),
		Annotations: map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": true},
		call:        (*Server).mcpCreateAlias,
	},
	{
		Name:        "list_inbox",
		Title:       "List inbox",
		Description: "List recent emails in an account's inbox. Each message has id, from, to, subject, date, a text preview and verification_code when a one-time code was detected.",
		InputSchema: mcpObjectSchema(map[string]any{
			"account_id": mcpAccountIDProp,
			"alias":      mcpAliasProp,
			"limit":      map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20, "description": "Maximum number of emails to return."},
			"days":       map[string]any{"type": "integer", "minimum": 1, "maximum": 90, "default": 7, "description": "Only include emails from the last N days."},
		}, "account_id"),
		Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": true},
		call:        (*Server).mcpListInbox,
	},
	{
		Name:        "get_verification_code",
		Title:       "Get latest verification code",
		Description: "Return the most recent verification code (OTP) found in an account's inbox, together with the email it came from. Returns found=false when no code was detected; the email may not have arrived yet, so wait a few seconds and call again.",
		InputSchema: mcpObjectSchema(map[string]any{
			"account_id": mcpAccountIDProp,
			"alias":      mcpAliasProp,
			"days":       map[string]any{"type": "integer", "minimum": 1, "maximum": 90, "default": 1, "description": "Only look at emails from the last N days."},
		}, "account_id"),
		Annotations: map[string]any{"readOnlyHint": true, "openWorldHint": true},
		call:        (*Server).mcpGetVerificationCode,
	},
}

func mcpArgError(message string) error {
	return &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: message}
}

// decodeMCPArgs 解析工具參數;用戶端可能省略 arguments 或傳 null。
func decodeMCPArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return mcpArgError("參數格式錯誤：" + err.Error())
	}
	return nil
}

// mcpIntArg 套用預設值並檢查範圍,與 REST 介面的限制一致。
func mcpIntArg(name string, v *int, def, min, max int) (int, error) {
	if v == nil {
		return def, nil
	}
	if *v < min || *v > max {
		return 0, mcpArgError(fmt.Sprintf("參數錯誤：%s 需為 %d-%d 的整數", name, min, max))
	}
	return *v, nil
}

func (s *Server) mcpListAccounts(json.RawMessage) (any, error) {
	accounts := s.be.ListAccounts()
	if accounts == nil {
		accounts = []account.Summary{}
	}
	return gin.H{"accounts": accounts}, nil
}

func (s *Server) mcpCreateAlias(raw json.RawMessage) (any, error) {
	var in struct {
		AccountID string `json:"account_id"`
		Label     string `json:"label"`
	}
	if err := decodeMCPArgs(raw, &in); err != nil {
		return nil, err
	}
	if in.AccountID == "" {
		return nil, mcpArgError("參數錯誤：account_id 必填")
	}
	if len([]rune(in.Label)) > 200 {
		return nil, mcpArgError("參數錯誤：label 最長 200 字元")
	}
	result, err := s.be.CreateAlias(in.AccountID, in.Label)
	if err != nil {
		return nil, err
	}
	return gin.H{"email": result.Email, "label": result.Label, "created_at": result.CreatedAt, "account_id": in.AccountID}, nil
}

type mcpInboxArgs struct {
	AccountID string `json:"account_id"`
	Alias     string `json:"alias"`
	Limit     *int   `json:"limit"`
	Days      *int   `json:"days"`
}

func (s *Server) mcpListInbox(raw json.RawMessage) (any, error) {
	var in mcpInboxArgs
	if err := decodeMCPArgs(raw, &in); err != nil {
		return nil, err
	}
	if in.AccountID == "" {
		return nil, mcpArgError("參數錯誤：account_id 必填")
	}
	limit, err := mcpIntArg("limit", in.Limit, 20, 1, 100)
	if err != nil {
		return nil, err
	}
	days, err := mcpIntArg("days", in.Days, 7, 1, 90)
	if err != nil {
		return nil, err
	}
	return s.be.ListInbox(InboxQuery{AccountID: in.AccountID, Alias: strings.TrimSpace(in.Alias), Limit: limit, Days: days})
}

func (s *Server) mcpGetVerificationCode(raw json.RawMessage) (any, error) {
	var in mcpInboxArgs
	if err := decodeMCPArgs(raw, &in); err != nil {
		return nil, err
	}
	if in.AccountID == "" {
		return nil, mcpArgError("參數錯誤：account_id 必填")
	}
	days, err := mcpIntArg("days", in.Days, 1, 1, 90)
	if err != nil {
		return nil, err
	}
	alias := strings.TrimSpace(in.Alias)
	result, err := s.be.ListInbox(InboxQuery{AccountID: in.AccountID, Alias: alias, Limit: 20, Days: days})
	if err != nil {
		return nil, err
	}
	out := gin.H{"account_id": in.AccountID, "alias": alias, "checked": len(result.Messages)}
	if m := newestWithCode(result.Messages); m != nil {
		out["found"], out["code"], out["message"] = true, m.VerificationCode, m
	} else {
		out["found"] = false
	}
	return out, nil
}

// newestWithCode 依日期挑出最新一封含驗證碼的郵件,不依賴上游回傳順序;
// 日期無法解析的郵件排在最後。
func newestWithCode(messages []mail.Message) *mail.Message {
	var best *mail.Message
	var bestAt time.Time
	for i := range messages {
		m := &messages[i]
		if m.VerificationCode == "" {
			continue
		}
		at, _ := time.Parse(time.RFC3339, m.Date)
		if best == nil || at.After(bestAt) {
			best, bestAt = m, at
		}
	}
	return best
}
