package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// 本文件锁定一条关键不变量:
//
//	HTTP 401 只表示"管理员会话失效",绝不能被上游 iCloud 的业务错误复用。
//
// 回归背景:上游鉴权失败(如 iCloud Cookie 过期、OTP 输错)曾经被映射为
// 401/UPSTREAM_UNAUTHORIZED,而前端 web/src/api/client.ts 收到任意 401 都会
// 清空会话并跳回登录页 —— 表现就是管理面板"一直被登出"。

// TestUpstreamUnauthorizedErrIsNot401 校验上游失效错误的构造结果。
func TestUpstreamUnauthorizedErrIsNot401(t *testing.T) {
	be := upstreamUnauthorizedErr()
	if be.Status == http.StatusUnauthorized {
		t.Fatalf("上游会话失效错误不得使用 401,得到 %d", be.Status)
	}
	if be.Status != http.StatusBadGateway {
		t.Fatalf("期望 502,得到 %d", be.Status)
	}
	if be.Code != "UPSTREAM_UNAUTHORIZED" {
		t.Fatalf("期望 code=UPSTREAM_UNAUTHORIZED,得到 %q", be.Code)
	}
}

// TestClassifyUpstreamErrNeverReturns401 覆盖真实的上游错误文本。
func TestClassifyUpstreamErrNeverReturns401(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"HTTP 401", errors.New("创建别名失败: HTTP 401: unauthorized"), "UPSTREAM_UNAUTHORIZED"},
		{"HTTP 403", errors.New("HTTP 403: forbidden"), "UPSTREAM_UNAUTHORIZED"},
		{"会话校验失败", errors.New("会话校验失败"), "UPSTREAM_UNAUTHORIZED"},
		{"cookie 缺失", errors.New("无 Cookie"), "UPSTREAM_UNAUTHORIZED"},
		{"普通上游故障", errors.New("HTTP 502: bad gateway"), "UPSTREAM_FAILURE"},
		{"DNS 故障", errors.New("dial tcp: no such host"), "UPSTREAM_FAILURE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := classifyUpstreamErr("操作失败", tc.err)
			if be == nil {
				t.Fatal("期望非空错误")
			}
			if be.Status == http.StatusUnauthorized {
				t.Fatalf("上游错误不得返回 401(err=%q)", tc.err)
			}
			if be.Code != tc.code {
				t.Fatalf("期望 code=%q,得到 %q", tc.code, be.Code)
			}
		})
	}
}

// TestClassifyLoginErrNeverReturns401 覆盖 iCloud 账号登录失败的各种文本。
func TestClassifyLoginErrNeverReturns401(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"OTP 错误", errors.New("2FA 验证失败: HTTP 401"), "OTP_INVALID"},
		{"需要 OTP", errors.New("账号启用了双重认证,需要提供 OTP"), "OTP_REQUIRED"},
		{"auth complete 401", errors.New("auth complete 失败: HTTP 401"), "UPSTREAM_UNAUTHORIZED"},
		{"trust 401", errors.New("trust 失败: HTTP 401"), "UPSTREAM_UNAUTHORIZED"},
		{"密码错误", errors.New("用户名或密码错误"), "UPSTREAM_FAILURE"},
		{"账号不存在", errors.New("账号不存在"), "ACCOUNT_NOT_FOUND"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := classifyLoginErr(tc.err)
			if be.Status == http.StatusUnauthorized {
				t.Fatalf("iCloud 登录错误不得返回 401(err=%q)", tc.err)
			}
			if be.Code != tc.code {
				t.Fatalf("期望 code=%q,得到 %q", tc.code, be.Code)
			}
		})
	}
}

// TestUpstreamFailureKeepsAdminSession 端到端验证:
// 一次上游 iCloud 失效不会 401,管理员会话保持有效(面板不被登出)。
func TestUpstreamFailureKeepsAdminSession(t *testing.T) {
	f := &fakeBackend{
		// 与 managerBackend.SetAliasActive 的真实行为一致
		aliasActErr: classifyUpstreamErr("停用失败", errors.New("HTTP 401: unauthorized")),
	}
	_, ts := newTestServer(f)
	defer ts.Close()

	cookie, csrf := login(t, ts, "admin-pass-2026-strong")

	// 1) 触发一次上游失效的操作
	req := authedReq(t, ts, http.MethodPost, "/api/aliases/anon_1/deactivate", `{"account_id":"acc_1"}`)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	req.Header.Set("X-CSRF-Token", csrf)

	status, raw, _ := do(t, req)
	if status == http.StatusUnauthorized {
		t.Fatalf("上游失效不得返回 401,响应: %s", raw)
	}
	if status != http.StatusBadGateway {
		t.Fatalf("期望 502,得到 %d,响应: %s", status, raw)
	}
	if !strings.Contains(raw, "UPSTREAM_UNAUTHORIZED") {
		t.Fatalf("期望 code=UPSTREAM_UNAUTHORIZED,响应: %s", raw)
	}

	// 2) 管理员会话必须仍然有效
	req2 := authedReq(t, ts, http.MethodGet, "/api/accounts", "")
	req2.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	status2, raw2, _ := do(t, req2)
	if status2 != http.StatusOK {
		t.Fatalf("上游失败后管理员会话应保持有效,得到 %d,响应: %s", status2, raw2)
	}
}
