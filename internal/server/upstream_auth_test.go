package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// 本檔案鎖定一條關鍵不變量:
//
//	HTTP 401 只表示"管理員工作階段失效",絕不能被上游 iCloud 的業務錯誤複用。
//
// 回歸背景:上游鑑權失敗(如 iCloud Cookie 過期、OTP 輸錯)曾經被映射為
// 401/UPSTREAM_UNAUTHORIZED,而前端 web/src/api/client.ts 收到任意 401 都會
// 清空工作階段並跳回登入頁 —— 表現就是管理面板"一直被登出"。

// TestUpstreamUnauthorizedErrIsNot401 校驗上游失效錯誤的構造結果。
func TestUpstreamUnauthorizedErrIsNot401(t *testing.T) {
	be := upstreamUnauthorizedErr()
	if be.Status == http.StatusUnauthorized {
		t.Fatalf("上游工作階段失效錯誤不得使用 401,得到 %d", be.Status)
	}
	if be.Status != http.StatusBadGateway {
		t.Fatalf("期望 502,得到 %d", be.Status)
	}
	if be.Code != "UPSTREAM_UNAUTHORIZED" {
		t.Fatalf("期望 code=UPSTREAM_UNAUTHORIZED,得到 %q", be.Code)
	}
}

// TestClassifyUpstreamErrNeverReturns401 覆蓋真實的上游錯誤文本。
func TestClassifyUpstreamErrNeverReturns401(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"HTTP 401", errors.New("建立別名失敗: HTTP 401: unauthorized"), "UPSTREAM_UNAUTHORIZED"},
		{"HTTP 403", errors.New("HTTP 403: forbidden"), "UPSTREAM_UNAUTHORIZED"},
		{"工作階段校驗失敗", errors.New("会话校验失败"), "UPSTREAM_UNAUTHORIZED"},
		{"cookie 缺失", errors.New("無 Cookie"), "UPSTREAM_UNAUTHORIZED"},
		{"普通上游故障", errors.New("HTTP 502: bad gateway"), "UPSTREAM_FAILURE"},
		{"DNS 故障", errors.New("dial tcp: no such host"), "UPSTREAM_FAILURE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := classifyUpstreamErr("操作失敗", tc.err)
			if be == nil {
				t.Fatal("期望非空錯誤")
			}
			if be.Status == http.StatusUnauthorized {
				t.Fatalf("上游錯誤不得返回 401(err=%q)", tc.err)
			}
			if be.Code != tc.code {
				t.Fatalf("期望 code=%q,得到 %q", tc.code, be.Code)
			}
		})
	}
}

// TestClassifyLoginErrNeverReturns401 覆蓋 iCloud 帳號登入失敗的各種文本。
func TestClassifyLoginErrNeverReturns401(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"OTP 錯誤（實際產生字串）", errors.New("2FA 驗證失敗：HTTP 401"), "OTP_INVALID"},
		{"OTP 錯誤（相容舊簡體字串）", errors.New("2FA 验证失败: HTTP 401"), "OTP_INVALID"},
		{"需要 OTP", errors.New("帳號已啟用雙重認證，需要提供 OTP"), "OTP_REQUIRED"},
		{"auth complete 401", errors.New("auth complete 失敗：HTTP 401"), "UPSTREAM_UNAUTHORIZED"},
		{"trust 401", errors.New("trust 失敗：HTTP 401"), "UPSTREAM_UNAUTHORIZED"},
		{"密碼錯誤", errors.New("使用者名稱或密碼錯誤"), "UPSTREAM_FAILURE"},
		{"帳號不存在", errors.New("帳號不存在"), "ACCOUNT_NOT_FOUND"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := classifyLoginErr(tc.err)
			if be.Status == http.StatusUnauthorized {
				t.Fatalf("iCloud 登入錯誤不得返回 401(err=%q)", tc.err)
			}
			if be.Code != tc.code {
				t.Fatalf("期望 code=%q,得到 %q", tc.code, be.Code)
			}
		})
	}
}

// TestUpstreamFailureKeepsAdminSession 端到端驗證:
// 一次上游 iCloud 失效不會 401,管理員工作階段保持有效(面板不被登出)。
func TestUpstreamFailureKeepsAdminSession(t *testing.T) {
	f := &fakeBackend{
		// 與 managerBackend.SetAliasActive 的真實行為一致
		aliasActErr: classifyUpstreamErr("停用失敗", errors.New("HTTP 401: unauthorized")),
	}
	_, ts := newTestServer(f)
	defer ts.Close()

	cookie, csrf := login(t, ts, "admin-pass-2026-strong")

	// 1) 觸發一次上游失效的操作
	req := authedReq(t, ts, http.MethodPost, "/api/aliases/anon_1/deactivate", `{"account_id":"acc_1"}`)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	req.Header.Set("X-CSRF-Token", csrf)

	status, raw, _ := do(t, req)
	if status == http.StatusUnauthorized {
		t.Fatalf("上游失效不得返回 401,回應: %s", raw)
	}
	if status != http.StatusBadGateway {
		t.Fatalf("期望 502,得到 %d,回應: %s", status, raw)
	}
	if !strings.Contains(raw, "UPSTREAM_UNAUTHORIZED") {
		t.Fatalf("期望 code=UPSTREAM_UNAUTHORIZED,回應: %s", raw)
	}

	// 2) 管理員工作階段必須仍然有效
	req2 := authedReq(t, ts, http.MethodGet, "/api/accounts", "")
	req2.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	status2, raw2, _ := do(t, req2)
	if status2 != http.StatusOK {
		t.Fatalf("上游失敗後管理員工作階段應保持有效,得到 %d,回應: %s", status2, raw2)
	}
}
