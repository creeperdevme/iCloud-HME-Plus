package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"icloud-hme/internal/account"
)

// writeSecretAccounts 把含秘密的帳號寫入測試資料目錄。
func writeSecretAccounts(t *testing.T, dir string) {
	t.Helper()
	data := `{
  "accounts": {
    "acc_secret": {
      "id": "acc_secret",
      "name": "秘密帳號",
      "real_email": "owner@example.com",
      "icloud_email": "owner@icloud.com",
      "cookies": {"X-APPLE-WEBAUTH-USER": "cookie-secret", "dsid": "dsid-secret"},
      "host": "icloud.com",
      "proxy": "http://user:proxy-secret@example.com:8080",
      "app_password": "app-secret",
      "status": "active",
      "alias_total": 3,
      "alias_active": 2,
      "last_validated": "2026-08-04T09:00:00+08:00",
      "created_at": "2026-08-01T09:00:00+08:00"
    }
  },
  "updated_at": "2026-08-04T09:00:00+08:00"
}`
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestAccountResponseNoSecrets 驗證帳號列表回應不包含任何秘密。
func TestAccountResponseNoSecrets(t *testing.T) {
	dir := t.TempDir()
	writeSecretAccounts(t, dir)
	mgr, err := account.NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := newWithBackend(&managerBackend{mgr: mgr}, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")
	req := authedReq(t, ts, "GET", "/api/accounts", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}

	// 位元組級斷言:不含秘密子串
	for _, secret := range []string{"cookie-secret", "app-secret", "proxy-secret", "dsid-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("回應洩露秘密 %q: %s", secret, body)
		}
	}

	// 精確鍵斷言:不存在 cookies / app_password / proxy 鍵
	var out struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("期望 1 個帳號,得到 %d", len(out.Data))
	}
	acc := out.Data[0]
	for _, forbidden := range []string{"cookies", "app_password", "proxy"} {
		if _, exists := acc[forbidden]; exists {
			t.Fatalf("回應包含禁止鍵 %q", forbidden)
		}
	}
	// 合法鍵存在
	for _, required := range []string{"has_cookies", "has_app_password", "has_proxy", "id", "name"} {
		if _, exists := acc[required]; !exists {
			t.Fatalf("回應缺少鍵 %q", required)
		}
	}
	if acc["has_cookies"] != true || acc["has_app_password"] != true || acc["has_proxy"] != true {
		t.Fatalf("憑證狀態錯誤: %v", acc)
	}
	_ = csrf
}

// TestAccountLoginResponseNoSecrets 驗證 iCloud 登入成功回應只含 Summary 欄位。
func TestAccountLoginResponseNoSecrets(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{{
			ID: "acc_secret", Name: "秘密帳號", Status: "active",
			HasCookies: true, HasAppPassword: true, HasProxy: true,
		}},
	}
	s := newWithBackend(f, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")
	req := authedReq(t, ts, "POST", "/api/accounts/acc_secret/login", `{"password":"p@ssw0rd-2026"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if _, exists := out.Data["cookies"]; exists {
		t.Fatalf("登入回應不應包含 cookies 鍵")
	}
	// 只允許 Summary 欄位
	for key := range out.Data {
		switch key {
		case "id", "name", "real_email", "icloud_email", "host", "status", "alias_total",
			"alias_active", "has_cookies", "has_app_password", "has_proxy",
			"last_validated", "status_message", "created_at":
		default:
			t.Fatalf("登入回應包含意外欄位 %q", key)
		}
	}
}

// TestAccountHandlerValidation 驗證帳號端點的參數校驗(使用真實 manager 適配器)。
func TestAccountHandlerValidation(t *testing.T) {
	dir := t.TempDir()
	mgr, err := account.NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := newWithBackend(&managerBackend{mgr: mgr}, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{"非法前綴", "POST", "/api/accounts", `{"icloud_email":"bad prefix"}`, 400, "VALIDATION_ERROR"},
		{"空信箱", "POST", "/api/accounts", `{"name":"主號","icloud_email":""}`, 400, "VALIDATION_ERROR"},
		{"非法主機", "POST", "/api/accounts", `{"name":"主號","icloud_email":"a@icloud.com","host":"evil.com"}`, 400, "VALIDATION_ERROR"},
		{"非法代理", "POST", "/api/accounts", `{"name":"主號","icloud_email":"a@icloud.com","proxy":"ftp://x"}`, 400, "VALIDATION_ERROR"},
		{"PATCH 空更新", "PATCH", "/api/accounts/acc_1", `{}`, 400, "VALIDATION_ERROR"},
		{"PUT 非法代理", "PUT", "/api/accounts/acc_1/proxy", `{"proxy":"ftp://x"}`, 400, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		req := authedReq(t, ts, tc.method, tc.path, tc.body)
		req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
		req.Header.Set("X-CSRF-Token", csrf)
		status, body, _ := do(t, req)
		if status != tc.status {
			t.Fatalf("%s: 期望 %d,得到 %d: %s", tc.name, tc.status, status, body)
		}
		var out struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal([]byte(body), &out)
		if out.Code != tc.code {
			t.Fatalf("%s: 期望 code=%s,得到 %q", tc.name, tc.code, out.Code)
		}
	}
}

// TestAccountUpdateCookiesAcceptString 驗證 PUT cookies 同時接受字串和物件。
func TestAccountUpdateCookiesAcceptString(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{{ID: "acc_1", Name: "主號"}}}
	s := newWithBackend(f, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	// 字串輸入
	req := authedReq(t, ts, "PUT", "/api/accounts/acc_1/cookies", `{"cookies":"a=1; b=2"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, _, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("字串 cookies 期望 200,得到 %d", status)
	}

	// 物件輸入
	req = authedReq(t, ts, "PUT", "/api/accounts/acc_1/cookies", `{"cookies":{"a":"1","b":"2"}}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, _, _ = do(t, req)
	if status != http.StatusOK {
		t.Fatalf("物件 cookies 期望 200,得到 %d", status)
	}
}

// TestAccountDeleteNotFound 驗證刪除不存在的帳號返回 404/ACCOUNT_NOT_FOUND。
func TestAccountDeleteNotFound(t *testing.T) {
	f := &fakeBackend{removedOK: false}
	s := newWithBackend(f, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")
	req := authedReq(t, ts, "DELETE", "/api/accounts/acc_missing", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, body, _ := do(t, req)
	if status != http.StatusNotFound {
		t.Fatalf("期望 404,得到 %d", status)
	}
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Code != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("期望 code=ACCOUNT_NOT_FOUND,得到 %q", out.Code)
	}
}

var _ = io.Discard

// newAccountTestServer 起一個帶指定 fake 的測試伺服器並登入。
func newAccountTestServer(t *testing.T, f *fakeBackend) (*httptest.Server, string, string) {
	t.Helper()
	s := newWithBackend(f, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	sess, csrf := login(t, ts, "admin-pass-2026-strong")
	return ts, sess, csrf
}

// postAuthed 送一個帶工作階段與 CSRF 的請求,回傳狀態碼與回應內容。
func postAuthed(t *testing.T, ts *httptest.Server, sess, csrf, method, path, body string) (int, string) {
	t.Helper()
	req := authedReq(t, ts, method, path, body)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, respBody, _ := do(t, req)
	return status, respBody
}

// TestAddAccountAcceptsAppPassword 驗證新增帳號時可以直接帶上 App 專用密碼。
func TestAddAccountAcceptsAppPassword(t *testing.T) {
	f := &fakeBackend{addHasAppPassword: true}
	ts, sess, csrf := newAccountTestServer(t, f)

	status, body := postAuthed(t, ts, sess, csrf, "POST", "/api/accounts",
		`{"icloud_email":"owner","app_password":"abcd-efgh-ijkl-mnop"}`)
	if status != http.StatusCreated {
		t.Fatalf("期望 201,得到 %d: %s", status, body)
	}
	if f.addedInput.AppPassword != "abcd-efgh-ijkl-mnop" {
		t.Fatalf("App 專用密碼沒有傳到 backend: %q", f.addedInput.AppPassword)
	}
	// 密碼有存進去,不該出現 warning。
	var out struct {
		Warning string `json:"warning"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Warning != "" {
		t.Fatalf("驗證成功不該有 warning,得到 %q", out.Warning)
	}
	// 回應不得洩露密碼本身。
	if strings.Contains(body, "abcd-efgh-ijkl-mnop") {
		t.Fatalf("回應洩露了 App 專用密碼: %s", body)
	}
}

// TestAddAccountWarnsWhenAppPasswordRejected 驗證密碼沒通過時仍建立帳號,
// 但以 warning 明確告知沒有存進去。
func TestAddAccountWarnsWhenAppPasswordRejected(t *testing.T) {
	f := &fakeBackend{addHasAppPassword: false}
	ts, sess, csrf := newAccountTestServer(t, f)

	status, body := postAuthed(t, ts, sess, csrf, "POST", "/api/accounts",
		`{"icloud_email":"owner","app_password":"wrong-password"}`)
	if status != http.StatusCreated {
		t.Fatalf("帳號仍應建立(201),得到 %d: %s", status, body)
	}
	var out struct {
		Success bool            `json:"success"`
		Warning string          `json:"warning"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("success 應為 true: %s", body)
	}
	if out.Warning == "" {
		t.Fatalf("密碼未儲存時應帶 warning: %s", body)
	}
	if len(out.Data) == 0 {
		t.Fatalf("仍應回傳帳號摘要: %s", body)
	}
}

// TestAddAccountWithoutAppPasswordHasNoWarning 驗證沒填密碼時不會多出 warning。
func TestAddAccountWithoutAppPasswordHasNoWarning(t *testing.T) {
	f := &fakeBackend{}
	ts, sess, csrf := newAccountTestServer(t, f)

	status, body := postAuthed(t, ts, sess, csrf, "POST", "/api/accounts", `{"icloud_email":"owner"}`)
	if status != http.StatusCreated {
		t.Fatalf("期望 201,得到 %d: %s", status, body)
	}
	if strings.Contains(body, "warning") {
		t.Fatalf("沒填密碼不該有 warning: %s", body)
	}
}

// TestSetAppPasswordFallsBackToStoredEmail 驗證既有帳號不必再填完整 iCloud 信箱。
func TestSetAppPasswordFallsBackToStoredEmail(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{{
		ID:          "acc_1",
		Name:        "主號",
		ICloudEmail: "owner@icloud.com",
	}}}
	ts, sess, csrf := newAccountTestServer(t, f)

	// 只送密碼,不送 icloud_email。
	status, body := postAuthed(t, ts, sess, csrf, "POST", "/api/accounts/acc_1/password",
		`{"app_password":"abcd-efgh-ijkl-mnop"}`)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}
	if f.appPwdID != "acc_1" {
		t.Fatalf("帳號 id 不正確: %q", f.appPwdID)
	}
	if f.appPwdEmail != "owner@icloud.com" {
		t.Fatalf("應沿用帳號已儲存的信箱,得到 %q", f.appPwdEmail)
	}
}

// TestSetAppPasswordExplicitEmailWins 驗證明確帶信箱時以帶入的為準。
func TestSetAppPasswordExplicitEmailWins(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{{
		ID:          "acc_1",
		Name:        "主號",
		ICloudEmail: "old@icloud.com",
	}}}
	ts, sess, csrf := newAccountTestServer(t, f)

	status, body := postAuthed(t, ts, sess, csrf, "POST", "/api/accounts/acc_1/password",
		`{"icloud_email":"new@icloud.com","app_password":"abcd-efgh-ijkl-mnop"}`)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}
	if f.appPwdEmail != "new@icloud.com" {
		t.Fatalf("應使用帶入的信箱,得到 %q", f.appPwdEmail)
	}
}

// TestSetAppPasswordErrors 驗證各種失敗情境的狀態碼與錯誤碼。
func TestSetAppPasswordErrors(t *testing.T) {
	cases := []struct {
		name    string
		fake    *fakeBackend
		path    string
		body    string
		status  int
		code    string
		wantMsg string
	}{
		{
			name:   "缺少 app_password",
			fake:   &fakeBackend{accounts: []account.Summary{{ID: "acc_1", ICloudEmail: "a@icloud.com"}}},
			path:   "/api/accounts/acc_1/password",
			body:   `{"icloud_email":"a@icloud.com"}`,
			status: 400, code: "VALIDATION_ERROR",
		},
		{
			name:   "帳號不存在",
			fake:   &fakeBackend{},
			path:   "/api/accounts/acc_missing/password",
			body:   `{"app_password":"abcd-efgh-ijkl-mnop"}`,
			status: 404, code: "ACCOUNT_NOT_FOUND",
		},
		{
			name:   "帳號沒有已存信箱又沒帶",
			fake:   &fakeBackend{accounts: []account.Summary{{ID: "acc_1", Name: "無信箱"}}},
			path:   "/api/accounts/acc_1/password",
			body:   `{"app_password":"abcd-efgh-ijkl-mnop"}`,
			status: 400, code: "VALIDATION_ERROR",
			// 訊息要引導使用者去填信箱,而不是只說參數錯誤。
			wantMsg: "完整信箱",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, sess, csrf := newAccountTestServer(t, tc.fake)
			status, body := postAuthed(t, ts, sess, csrf, "POST", tc.path, tc.body)
			if status != tc.status {
				t.Fatalf("期望 %d,得到 %d: %s", tc.status, status, body)
			}
			var out struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal([]byte(body), &out); err != nil {
				t.Fatal(err)
			}
			if out.Code != tc.code {
				t.Fatalf("期望 code=%s,得到 %q", tc.code, out.Code)
			}
			if tc.wantMsg != "" && !strings.Contains(out.Message, tc.wantMsg) {
				t.Fatalf("訊息應包含 %q,得到 %q", tc.wantMsg, out.Message)
			}
		})
	}
}
