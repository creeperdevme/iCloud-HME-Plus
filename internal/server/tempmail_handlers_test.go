package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/tempmail"
)

// tempResp 是隨機信箱端點的回應外層。
type tempResp struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		AccountID string `json:"account_id"`
		Label     string `json:"label"`
		CreatedAt string `json:"created_at"`
		ExpiresAt string `json:"expires_at"`
		Keep      bool   `json:"keep"`
		Count     int    `json:"count"`
		TTLSecond int    `json:"ttl_seconds"`
		Mailboxes []struct {
			ID        string `json:"id"`
			Email     string `json:"email"`
			Keep      bool   `json:"keep"`
			ExpiresAt string `json:"expires_at"`
		} `json:"mailboxes"`
		Removed         bool   `json:"removed"`
		UpstreamWarning string `json:"upstream_warning"`
	} `json:"data"`
}

// tempServer 建立測試 Server、登入並回傳已帶 Cookie 的請求工廠。
func tempServer(t *testing.T, f *fakeBackend, cfg Config) (*Server, *httptest.Server, func(string, string, string) *http.Request) {
	t.Helper()
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = "admin-pass-2026-strong"
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	s := newWithBackend(f, cfg)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	sess, csrf := login(t, ts, cfg.AdminPassword)
	factory := func(method, path, body string) *http.Request {
		req := authedReq(t, ts, method, path, body)
		req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
		if method != http.MethodGet {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		return req
	}
	return s, ts, factory
}

func decodeTemp(t *testing.T, body string) tempResp {
	t.Helper()
	var out tempResp
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("解析回應失敗：%v（原始內容 %s）", err, body)
	}
	return out
}

// readyAccount 是一個憑證齊全、可用來建立隨機信箱的帳號。
func readyAccount(id, name string) account.Summary {
	return account.Summary{ID: id, Name: name, HasCookies: true}
}

func TestCreateTempMailbox(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{readyAccount("acc_1", "主要帳號")},
		created: &hme.CreateResult{
			Email: "jade.reef.4821@icloud.com", Label: "temp-jade-reef-4821",
			AnonymousID: "anon_1", CreatedAt: time.Now().Format(time.RFC3339),
		},
	}
	s, _, req := tempServer(t, f, Config{})
	fixed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return fixed }

	status, body, _ := do(t, req("POST", "/api/temp", `{}`))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	out := decodeTemp(t, body)
	if !out.Success {
		t.Fatalf("回應應為成功：%s", body)
	}
	if out.Data.ID != "anon_1" || out.Data.Email != "jade.reef.4821@icloud.com" {
		t.Errorf("回傳內容不符：%+v", out.Data)
	}
	if out.Data.AccountID != "acc_1" {
		t.Errorf("account_id = %s，期望 acc_1", out.Data.AccountID)
	}
	if out.Data.Keep {
		t.Error("新建的隨機信箱預設不應是「不自動刪除」")
	}
	// 預設 TTL 24 小時。
	wantExpiry := fixed.Add(24 * time.Hour)
	got, err := time.Parse(time.RFC3339Nano, out.Data.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at 無法解析：%v", err)
	}
	if !got.UTC().Equal(wantExpiry) {
		t.Errorf("expires_at = %v，期望 %v", got.UTC(), wantExpiry)
	}
	// 標籤應是可辨識的隨機標籤，且確實送到後端。
	if !strings.HasPrefix(f.createdLabel, "temp-") {
		t.Errorf("標籤 %q 應以 temp- 開頭", f.createdLabel)
	}
	if f.createdLabel != out.Data.Label {
		t.Errorf("回傳標籤 %q 與實際送到後端的 %q 不一致", out.Data.Label, f.createdLabel)
	}
	if f.createdAccountID != "acc_1" {
		t.Errorf("建立時使用的帳號 = %s，期望 acc_1", f.createdAccountID)
	}
	if s.temp.Len() != 1 {
		t.Errorf("追蹤數量 = %d，期望 1", s.temp.Len())
	}
}

// TestCreateTempMailboxEmptyBody 確認完全不帶 body 也能建立：
// 只有一個帳號時前端不需要傳 account_id，可能直接送空請求。
func TestCreateTempMailboxEmptyBody(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{readyAccount("acc_1", "主要")},
		created:  &hme.CreateResult{Email: "a@icloud.com", AnonymousID: "anon_1"},
	}
	_, _, req := tempServer(t, f, Config{})

	for _, body := range []string{"", "{}"} {
		f.createdLabel = ""
		status, resp, _ := do(t, req("POST", "/api/temp", body))
		if status != http.StatusOK {
			t.Fatalf("body=%q 應可建立，得到 %d：%s", body, status, resp)
		}
		if f.createdAccountID != "acc_1" {
			t.Errorf("body=%q 應使用唯一可用帳號，實際 %q", body, f.createdAccountID)
		}
		if f.createdLabel == "" {
			t.Errorf("body=%q 應該有帶標籤去建立", body)
		}
	}
}

func TestCreateTempMailboxNoUsableAccount(t *testing.T) {
	cases := []struct {
		name     string
		accounts []account.Summary
	}{
		{"完全沒有帳號", nil},
		{"只有沒有憑證的帳號", []account.Summary{{ID: "acc_1", Name: "空帳號"}}},
		// 只有 App 專用密碼不足以建立 HME 別名：HME 介面需要 Cookie。
		{"只有 App 專用密碼的帳號", []account.Summary{{ID: "acc_1", Name: "只有密碼", HasAppPassword: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeBackend{accounts: tc.accounts}
			_, _, req := tempServer(t, f, Config{})

			status, body, _ := do(t, req("POST", "/api/temp", `{}`))
			if status != http.StatusBadRequest {
				t.Fatalf("期望 400，得到 %d：%s", status, body)
			}
			out := decodeTemp(t, body)
			if out.Code != "VALIDATION_ERROR" {
				t.Errorf("錯誤碼 = %s，期望 VALIDATION_ERROR", out.Code)
			}
			if !strings.Contains(out.Message, "帳號管理") {
				t.Errorf("訊息應引導使用者去新增帳號，得到 %q", out.Message)
			}
			// 不應該真的去建立別名。
			if f.createdLabel != "" {
				t.Errorf("沒有可用帳號時不應呼叫建立，卻帶了標籤 %q", f.createdLabel)
			}
		})
	}
}

func TestCreateTempMailboxAccountSelection(t *testing.T) {
	// 只有 App 專用密碼的帳號不能建立別名，必須跳過它去用有 Cookie 的帳號，
	// 否則會選到一個註定失敗的帳號。
	t.Run("未指定時跳過沒有 Cookie 的帳號", func(t *testing.T) {
		f := &fakeBackend{
			accounts: []account.Summary{
				{ID: "acc_app_pwd", Name: "只有 App 密碼", HasAppPassword: true},
				{ID: "acc_empty", Name: "沒設定"},
				{ID: "acc_cookies", Name: "有 Cookie", HasCookies: true},
			},
			created: &hme.CreateResult{Email: "a@icloud.com", AnonymousID: "anon_a"},
		}
		_, _, req := tempServer(t, f, Config{})
		if status, body, _ := do(t, req("POST", "/api/temp", `{}`)); status != http.StatusOK {
			t.Fatalf("期望 200，得到 %d：%s", status, body)
		}
		if f.createdAccountID != "acc_cookies" {
			t.Errorf("選到的帳號 = %s，期望 acc_cookies", f.createdAccountID)
		}
	})

	t.Run("指定帳號時使用指定的", func(t *testing.T) {
		f := &fakeBackend{
			accounts: []account.Summary{
				readyAccount("acc_1", "第一個"),
				readyAccount("acc_2", "第二個"),
			},
			created: &hme.CreateResult{Email: "b@icloud.com", AnonymousID: "anon_b"},
		}
		_, _, req := tempServer(t, f, Config{})
		if status, body, _ := do(t, req("POST", "/api/temp", `{"account_id":"acc_2"}`)); status != http.StatusOK {
			t.Fatalf("期望 200，得到 %d：%s", status, body)
		}
		if f.createdAccountID != "acc_2" {
			t.Errorf("選到的帳號 = %s，期望 acc_2", f.createdAccountID)
		}
	})

	t.Run("指定不存在的帳號回 404", func(t *testing.T) {
		f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "唯一")}}
		_, _, req := tempServer(t, f, Config{})
		status, body, _ := do(t, req("POST", "/api/temp", `{"account_id":"acc_nope"}`))
		if status != http.StatusNotFound {
			t.Fatalf("期望 404，得到 %d：%s", status, body)
		}
		if got := decodeTemp(t, body).Code; got != "ACCOUNT_NOT_FOUND" {
			t.Errorf("錯誤碼 = %s，期望 ACCOUNT_NOT_FOUND", got)
		}
	})

	t.Run("指定沒有 Cookie 的帳號回 400 並說明原因", func(t *testing.T) {
		f := &fakeBackend{accounts: []account.Summary{{ID: "acc_empty", Name: "空帳號"}}}
		_, _, req := tempServer(t, f, Config{})
		status, body, _ := do(t, req("POST", "/api/temp", `{"account_id":"acc_empty"}`))
		if status != http.StatusBadRequest {
			t.Fatalf("期望 400，得到 %d：%s", status, body)
		}
		out := decodeTemp(t, body)
		if !strings.Contains(out.Message, "空帳號") || !strings.Contains(out.Message, "Cookie") {
			t.Errorf("訊息應指出是哪個帳號且說明缺 Cookie，得到 %q", out.Message)
		}
		if f.createdLabel != "" {
			t.Error("不該對沒有 Cookie 的帳號嘗試建立別名")
		}
	})
}

// TestCreateTempMailboxWithoutAnonymousID 確認拿不到別名識別碼時不會留下
// 一個永遠刪不掉的追蹤記錄，並且回報 502（而不是 401，避免前端被登出）。
func TestCreateTempMailboxWithoutAnonymousID(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{readyAccount("acc_1", "主要")},
		created:  &hme.CreateResult{Email: "x@icloud.com"},
	}
	s, _, req := tempServer(t, f, Config{})

	status, body, _ := do(t, req("POST", "/api/temp", `{}`))
	if status != http.StatusBadGateway {
		t.Fatalf("期望 502，得到 %d：%s", status, body)
	}
	if status == http.StatusUnauthorized {
		t.Fatal("絕不能用 401：前端收到 401 會清掉管理員工作階段")
	}
	if out := decodeTemp(t, body); out.Code != "UPSTREAM_FAILURE" {
		t.Errorf("錯誤碼 = %s，期望 UPSTREAM_FAILURE", out.Code)
	}
	if s.temp.Len() != 0 {
		t.Errorf("不該追蹤沒有識別碼的信箱，卻有 %d 筆", s.temp.Len())
	}
}

// TestCreateTempMailboxUpstreamFailure 確認上游失敗時回傳 502，管理員不會被登出。
func TestCreateTempMailboxUpstreamFailure(t *testing.T) {
	f := &fakeBackend{
		accounts:   []account.Summary{readyAccount("acc_1", "主要")},
		createdErr: &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "建立信箱失敗：iCloud 暫時無法使用"},
	}
	s, _, req := tempServer(t, f, Config{})

	status, body, _ := do(t, req("POST", "/api/temp", `{}`))
	if status != http.StatusBadGateway {
		t.Fatalf("期望 502，得到 %d：%s", status, body)
	}
	if s.temp.Len() != 0 {
		t.Errorf("失敗時不應留下追蹤記錄，卻有 %d 筆", s.temp.Len())
	}
	// 工作階段必須仍然有效。
	status, _, _ = do(t, req("GET", "/api/temp", ""))
	if status != http.StatusOK {
		t.Fatalf("上游失敗後管理員工作階段應仍有效，得到 %d", status)
	}
}

func TestListTempMailboxes(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	cfg := Config{TempTTL: 6 * time.Hour}
	s, _, req := tempServer(t, f, cfg)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return now }
	s.temp.Add(tempMailbox("anon_1", "a@icloud.com", now, now.Add(6*time.Hour), false))
	s.temp.Add(tempMailbox("anon_2", "b@icloud.com", now, time.Time{}, true))

	status, body, _ := do(t, req("GET", "/api/temp", ""))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	out := decodeTemp(t, body)
	if out.Data.Count != 2 || len(out.Data.Mailboxes) != 2 {
		t.Fatalf("應有 2 筆，得到 count=%d len=%d", out.Data.Count, len(out.Data.Mailboxes))
	}
	if out.Data.TTLSecond != int((6 * time.Hour).Seconds()) {
		t.Errorf("ttl_seconds = %d，期望 %d", out.Data.TTLSecond, int((6 * time.Hour).Seconds()))
	}
	// 不要外洩帳號的祕密欄位。
	for _, secret := range []string{"cookies", "app_password", "proxy"} {
		if strings.Contains(body, secret) {
			t.Errorf("回應不應包含 %q：%s", secret, body)
		}
	}
}

func TestListTempMailboxesEmptyIsArray(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	_, _, req := tempServer(t, f, Config{})

	_, body, _ := do(t, req("GET", "/api/temp", ""))
	// 空清單必須是 []，不能是 null，否則前端要另外防禦。
	if !strings.Contains(body, `"mailboxes":[]`) {
		t.Errorf("空清單應序列化為 []，得到 %s", body)
	}
}

func TestSetTempMailboxKeep(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	cfg := Config{TempTTL: 24 * time.Hour}
	s, _, req := tempServer(t, f, cfg)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return now }
	s.temp.Add(tempMailbox("anon_1", "a@icloud.com", now, now.Add(24*time.Hour), false))

	// 設為不自動刪除：到期時間應被清掉。
	status, body, _ := do(t, req("POST", "/api/temp/anon_1/keep", `{"keep":true}`))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	out := decodeTemp(t, body)
	if !out.Data.Keep {
		t.Error("keep 應為 true")
	}
	parsed, err := time.Parse(time.RFC3339Nano, out.Data.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at 無法解析：%v", err)
	}
	if !parsed.IsZero() {
		t.Errorf("不自動刪除時 expires_at 應為零值，得到 %q", out.Data.ExpiresAt)
	}

	// 再關掉：應從現在重新起算 24 小時，而不是沿用舊時間。
	later := now.Add(30 * time.Hour)
	s.clock = func() time.Time { return later }
	status, body, _ = do(t, req("POST", "/api/temp/anon_1/keep", `{"keep":false}`))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	out = decodeTemp(t, body)
	if out.Data.Keep {
		t.Error("keep 應為 false")
	}
	got, err := time.Parse(time.RFC3339Nano, out.Data.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at 無法解析：%v", err)
	}
	if !got.UTC().Equal(later.Add(24 * time.Hour)) {
		t.Errorf("expires_at = %v，期望 %v（應重新起算）", got.UTC(), later.Add(24*time.Hour))
	}
}

func TestSetTempMailboxKeepValidation(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	s, _, req := tempServer(t, f, Config{})
	now := time.Now()
	s.temp.Add(tempMailbox("anon_1", "a@icloud.com", now, now.Add(time.Hour), false))

	t.Run("缺少 keep 參數", func(t *testing.T) {
		for _, body := range []string{`{}`, `{"keep":null}`, `not-json`} {
			status, resp, _ := do(t, req("POST", "/api/temp/anon_1/keep", body))
			if status != http.StatusBadRequest {
				t.Errorf("body=%s 期望 400，得到 %d：%s", body, status, resp)
			}
		}
	})

	t.Run("找不到隨機信箱", func(t *testing.T) {
		status, body, _ := do(t, req("POST", "/api/temp/anon_missing/keep", `{"keep":true}`))
		if status != http.StatusNotFound {
			t.Fatalf("期望 404，得到 %d：%s", status, body)
		}
	})
}

func TestDeleteTempMailbox(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	s, _, req := tempServer(t, f, Config{})
	now := time.Now()
	s.temp.Add(tempMailbox("anon_1", "a@icloud.com", now, now.Add(time.Hour), false))

	status, body, _ := do(t, req("DELETE", "/api/temp/anon_1", ""))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	out := decodeTemp(t, body)
	if !out.Data.Removed || out.Data.Email != "a@icloud.com" {
		t.Errorf("回應不符：%+v", out.Data)
	}
	if f.aliasDeleteID != "anon_1" {
		t.Errorf("應在上游刪除 anon_1，實際 %q", f.aliasDeleteID)
	}
	if s.temp.Len() != 0 {
		t.Errorf("刪除後不應繼續追蹤，卻有 %d 筆", s.temp.Len())
	}
}

// TestDeleteTempMailboxUpstreamFailureStillUntracks 確認上游刪除失敗時仍然
// 停止追蹤：使用者已經按了刪除，而且若別名在別名頁被刪過，這裡必然失敗。
func TestDeleteTempMailboxUpstreamFailureStillUntracks(t *testing.T) {
	f := &fakeBackend{
		accounts:       []account.Summary{readyAccount("acc_1", "主要")},
		aliasDeleteErr: fmt.Errorf("上游：找不到這個別名"),
	}
	s, _, req := tempServer(t, f, Config{})
	now := time.Now()
	s.temp.Add(tempMailbox("anon_1", "a@icloud.com", now, now.Add(time.Hour), false))

	status, body, _ := do(t, req("DELETE", "/api/temp/anon_1", ""))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	out := decodeTemp(t, body)
	if !out.Data.Removed {
		t.Error("仍應回報已移除追蹤")
	}
	if out.Data.UpstreamWarning == "" {
		t.Error("應帶上 upstream_warning 讓前端提示使用者")
	}
	if s.temp.Len() != 0 {
		t.Errorf("應停止追蹤，卻還有 %d 筆", s.temp.Len())
	}
}

func TestDeleteTempMailboxNotFound(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	_, _, req := tempServer(t, f, Config{})

	status, body, _ := do(t, req("DELETE", "/api/temp/anon_missing", ""))
	if status != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d：%s", status, body)
	}
	if code := decodeTemp(t, body).Code; code != "NOT_FOUND" {
		t.Errorf("錯誤碼 = %s，期望 NOT_FOUND", code)
	}
}

// TestSweepTempOnceDeletesExpired 是這個功能的核心：到期就自動刪除。
func TestSweepTempOnceDeletesExpired(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	cfg := Config{TempTTL: 24 * time.Hour}
	s, _, _ := tempServer(t, f, cfg)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	s.temp.Add(tempMailbox("anon_expired", "old@icloud.com", now.Add(-25*time.Hour), now.Add(-time.Hour), false))
	s.temp.Add(tempMailbox("anon_fresh", "new@icloud.com", now, now.Add(23*time.Hour), false))
	s.temp.Add(tempMailbox("anon_kept", "kept@icloud.com", now.Add(-48*time.Hour), now.Add(-24*time.Hour), true))

	s.clock = func() time.Time { return now }
	s.sweepTempOnce()

	if len(f.aliasDeleteCalls) != 1 || f.aliasDeleteCalls[0] != "anon_expired" {
		t.Fatalf("應只刪除 anon_expired，實際呼叫 %v", f.aliasDeleteCalls)
	}
	if s.temp.Len() != 2 {
		t.Fatalf("應剩下 2 筆，得到 %d", s.temp.Len())
	}
	if _, found := s.temp.Get("anon_expired"); found {
		t.Error("已到期的記錄應該被移除")
	}
	if _, found := s.temp.Get("anon_kept"); !found {
		t.Error("標記為不自動刪除的信箱不該被刪掉")
	}
	if _, found := s.temp.Get("anon_fresh"); !found {
		t.Error("還沒到期的信箱不該被刪掉")
	}
}

// TestSweepTempOnceRetriesThenGivesUp 確認上游持續失敗時會重試，
// 但不會無限期地卡在清單裡。
func TestSweepTempOnceRetriesThenGivesUp(t *testing.T) {
	f := &fakeBackend{
		accounts:       []account.Summary{readyAccount("acc_1", "主要")},
		aliasDeleteErr: fmt.Errorf("上游持續失敗"),
	}
	s, _, _ := tempServer(t, f, Config{})
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return now }
	s.temp.Add(tempMailbox("anon_1", "a@icloud.com", now.Add(-25*time.Hour), now.Add(-time.Hour), false))

	// 前幾次應該保留記錄並持續重試。
	for i := 1; i < maxTempDeleteAttempts; i++ {
		s.sweepTempOnce()
		mb, found := s.temp.Get("anon_1")
		if !found {
			t.Fatalf("第 %d 次失敗後不應放棄", i)
		}
		if mb.Attempts != i {
			t.Fatalf("第 %d 次失敗後 Attempts = %d", i, mb.Attempts)
		}
		if mb.LastError == "" {
			t.Error("應記錄最後一次失敗原因")
		}
	}
	// 達到上限後放棄追蹤。
	s.sweepTempOnce()
	if _, found := s.temp.Get("anon_1"); found {
		t.Errorf("連續失敗 %d 次後應停止追蹤", maxTempDeleteAttempts)
	}
}

// TestTempMailboxPersistedToDataDir 確認 Config.DataDir 會讓追蹤記錄落地，
// 否則重啟後已建立的隨機信箱永遠不會被自動刪除。
func TestTempMailboxPersistedToDataDir(t *testing.T) {
	dir := t.TempDir()
	f := &fakeBackend{
		accounts: []account.Summary{readyAccount("acc_1", "主要")},
		created:  &hme.CreateResult{Email: "a@icloud.com", AnonymousID: "anon_1"},
	}
	s, _, req := tempServer(t, f, Config{DataDir: dir})
	if status, body, _ := do(t, req("POST", "/api/temp", `{}`)); status != http.StatusOK {
		t.Fatalf("建立失敗 %d：%s", status, body)
	}
	if s.temp.Len() != 1 {
		t.Fatalf("追蹤數量 = %d，期望 1", s.temp.Len())
	}

	// 用同一個資料目錄重新開一個 Server，應該要能載入剛才那筆。
	s2, _, req2 := tempServer(t, &fakeBackend{}, Config{DataDir: dir})
	if s2.temp.Len() != 1 {
		t.Fatalf("重啟後追蹤數量 = %d，期望 1", s2.temp.Len())
	}
	if _, found := s2.temp.Get("anon_1"); !found {
		t.Error("重啟後應該還能追蹤 anon_1")
	}
	status, body, _ := do(t, req2("GET", "/api/temp", ""))
	if status != http.StatusOK {
		t.Fatalf("期望 200，得到 %d：%s", status, body)
	}
	if out := decodeTemp(t, body); out.Data.Count != 1 || out.Data.Mailboxes[0].Email != "a@icloud.com" {
		t.Errorf("重啟後列表不符：%+v", out.Data)
	}
}

// TestTempMailboxRoutesRequireSession 確認隨機信箱端點仍遵守
// 「401 只代表管理員工作階段失效」這個不變式。
func TestTempMailboxRoutesRequireSession(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	_, ts, _ := tempServer(t, f, Config{})

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/temp", `{}`},
		{"GET", "/api/temp", ""},
		{"POST", "/api/temp/anon_1/keep", `{"keep":true}`},
		{"DELETE", "/api/temp/anon_1", ""},
	} {
		req := authedReq(t, ts, tc.method, tc.path, tc.body)
		status, body, _ := do(t, req)
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s 未帶工作階段應回 401，得到 %d：%s", tc.method, tc.path, status, body)
		}
	}
}

func TestTempMailboxWritesRequireCSRF(t *testing.T) {
	f := &fakeBackend{accounts: []account.Summary{readyAccount("acc_1", "主要")}}
	_, ts, _ := tempServer(t, f, Config{})
	sess, _ := login(t, ts, "admin-pass-2026-strong")

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/temp", `{}`},
		{"POST", "/api/temp/anon_1/keep", `{"keep":true}`},
		{"DELETE", "/api/temp/anon_1", ""},
	} {
		req := authedReq(t, ts, tc.method, tc.path, tc.body)
		req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
		status, body, _ := do(t, req)
		if status == http.StatusOK {
			t.Errorf("%s %s 沒有 CSRF 頭不應成功：%s", tc.method, tc.path, body)
		}
	}
}

func TestRandomTempLabel(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		label := randomTempLabel()
		if !strings.HasPrefix(label, "temp-") {
			t.Fatalf("標籤 %q 應以 temp- 開頭", label)
		}
		if len([]rune(label)) > 200 {
			t.Fatalf("標籤 %q 超過 200 字元上限", label)
		}
		parts := strings.Split(label, "-")
		if len(parts) != 4 {
			t.Fatalf("標籤 %q 格式應為 temp-<word>-<word>-<4 位數>", label)
		}
		if len(parts[3]) != 4 {
			t.Errorf("標籤 %q 的數字部分應補零到 4 位", label)
		}
		seen[label] = true
	}
	// 200 次裡幾乎不該有重複。
	if len(seen) < 195 {
		t.Errorf("200 次只產生 %d 個不同標籤，隨機性不足", len(seen))
	}
}

// TestTempStoreDegradesWhenFileCorrupt 確認記錄檔損毀時服務仍能啟動。
func TestTempStoreDegradesWhenFileCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tempmail.FileName), []byte("{ 壞掉的 JSON"), 0o600); err != nil {
		t.Fatalf("準備測試檔失敗：%v", err)
	}
	s := newWithBackend(&fakeBackend{}, Config{DataDir: dir})
	if s.temp == nil {
		t.Fatal("記錄檔損毀時應降級為記憶體追蹤，而不是讓 Server 建不起來")
	}
	if s.temp.Len() != 0 {
		t.Errorf("降級後應為空，得到 %d 筆", s.temp.Len())
	}
}

// tempMailbox 是測試用的隨機信箱記錄。
func tempMailbox(id, email string, created, expires time.Time, keep bool) tempmail.Mailbox {
	return tempmail.Mailbox{
		ID: id, Email: email, AccountID: "acc_1", Label: "temp-test-" + id,
		CreatedAt: created, ExpiresAt: expires, Keep: keep,
	}
}
