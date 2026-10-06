package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAuthRequiresSession 驗證不帶 Cookie 的 API 返回 401/AUTH_REQUIRED。
func TestAuthRequiresSession(t *testing.T) {
	f := &fakeBackend{}
	s, ts := newTestServer(f)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/accounts", nil)
	status, body, _ := do(t, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("期望 401,得到 %d", status)
	}
	var out struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != "AUTH_REQUIRED" {
		t.Fatalf("期望 code=AUTH_REQUIRED,得到 %q", out.Code)
	}
	_ = s
}

// TestAuthWrongPassword 驗證錯誤密碼返回 401/INVALID_CREDENTIALS 且不設定 Cookie。
func TestAuthWrongPassword(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	status, body, cookies := do(t, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("期望 401,得到 %d", status)
	}
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("期望 code=INVALID_CREDENTIALS,得到 %q", out.Code)
	}
	for _, c := range cookies {
		if c.Name == "hme_session" {
			t.Fatal("錯誤密碼不應設定 hme_session Cookie")
		}
	}
}

// TestAuthLoginSuccess 驗證正確登入設定 HttpOnly/SameSite=Strict Cookie。
func TestAuthLoginSuccess(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader(`{"password":"admin-pass-2026-strong"}`))
	req.Header.Set("Content-Type", "application/json")
	status, body, cookies := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}
	var out struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
			ExpiresAt string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.CSRFToken == "" {
		t.Fatal("回應應包含 csrf_token")
	}
	if _, err := time.Parse(time.RFC3339, out.Data.ExpiresAt); err != nil {
		t.Fatalf("expires_at 應為 RFC3339: %v", err)
	}
	var sess *http.Cookie
	for _, c := range cookies {
		if c.Name == "hme_session" {
			sess = c
		}
	}
	if sess == nil {
		t.Fatal("登入成功應設定 hme_session Cookie")
	}
	if !sess.HttpOnly {
		t.Fatal("hme_session 應設定 HttpOnly")
	}
	if sess.SameSite != http.SameSiteStrictMode {
		t.Fatalf("hme_session SameSite 應為 Strict,得到 %v", sess.SameSite)
	}
	if sess.Path != "/" {
		t.Fatalf("hme_session Path 應為 /,得到 %q", sess.Path)
	}
}

// TestAuthSessionFlow 驗證 Cookie 有效時 GET 成功;POST 需要 CSRF。
func TestAuthSessionFlow(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	// 帶 Cookie 的 GET 成功
	req := authedReq(t, ts, "GET", "/api/accounts", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("帶 Cookie 的 GET 期望 200,得到 %d: %s", status, body)
	}

	// 無 CSRF 的 POST 返回 403/CSRF_INVALID
	req = authedReq(t, ts, "POST", "/api/accounts", `{"name":"x","icloud_email":"a@icloud.com"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	status, body, _ = do(t, req)
	if status != http.StatusForbidden {
		t.Fatalf("無 CSRF 的 POST 期望 403,得到 %d: %s", status, body)
	}
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Code != "CSRF_INVALID" {
		t.Fatalf("期望 code=CSRF_INVALID,得到 %q", out.Code)
	}

	// 帶正確 CSRF 頭成功
	req = authedReq(t, ts, "POST", "/api/accounts", `{"name":"x","icloud_email":"a@icloud.com"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, _, _ = do(t, req)
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("帶 CSRF 的 POST 期望 2xx,得到 %d", status)
	}
}

// TestAuthLogout 驗證退出後同一 Cookie 立即失效。
func TestAuthLogout(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	req := authedReq(t, ts, "POST", "/api/auth/logout", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, _, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("退出期望 200,得到 %d", status)
	}

	// 同一 Cookie 立即失效
	req = authedReq(t, ts, "GET", "/api/accounts", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	status, _, _ = do(t, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("退出後 GET 期望 401,得到 %d", status)
	}
}

// TestAuthRateLimit 驗證同 IP 連續 6 次失敗後第 6 次返回 429/RATE_LIMITED。
func TestAuthRateLimit(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	var lastStatus int
	var lastBody string
	for i := 0; i < 6; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		lastStatus, lastBody, _ = do(t, req)
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("第 6 次失敗期望 429,得到 %d: %s", lastStatus, lastBody)
	}
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(lastBody), &out)
	if out.Code != "RATE_LIMITED" {
		t.Fatalf("期望 code=RATE_LIMITED,得到 %q", out.Code)
	}
}

// TestAuthSessionEndpoint 驗證 GET /api/auth/session 返回 CSRF 與過期時間。
func TestAuthSessionEndpoint(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	sess, _ := login(t, ts, "admin-pass-2026-strong")
	req := authedReq(t, ts, "GET", "/api/auth/session", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d", status)
	}
	var out struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
			ExpiresAt string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.CSRFToken == "" {
		t.Fatal("session 端點應返回 csrf_token")
	}
}

// TestAuthSessionEndpointInvalid 驗證無效工作階段的 session 端點返回 401。
func TestAuthSessionEndpointInvalid(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()

	req := authedReq(t, ts, "GET", "/api/auth/session", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: "invalid-session"})
	status, _, _ := do(t, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("無效工作階段期望 401,得到 %d", status)
	}
}
