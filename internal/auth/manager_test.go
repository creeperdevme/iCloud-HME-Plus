package auth

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
	"time"
)

// fixedNow 固定當前時間,便於測試過期。
var fixedNow = time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

// fixedRandom 確定性隨機源(用於產生 session ID / CSRF / salt)。
type fixedRandom struct {
	seq uint64
}

func (r *fixedRandom) Read(p []byte) (int, error) {
	for i := range p {
		r.seq++
		p[i] = byte(r.seq >> (8 * (uint(i) % 8)))
	}
	return len(p), nil
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	opts := Options{
		Password: "admin-pass-2026-strong",
		TTL:      12 * time.Hour,
		Now:      func() time.Time { return fixedNow },
		Random:   &fixedRandom{},
	}
	m, err := NewManager(opts)
	if err != nil {
		t.Fatalf("建立 Manager 失敗: %v", err)
	}
	return m
}

// TestManagerWrongPassword 驗證錯誤密碼被拒絕。
func TestManagerWrongPassword(t *testing.T) {
	m := newTestManager(t)
	if _, _, ok := m.Login("wrong-password"); ok {
		t.Fatal("錯誤密碼不應登入成功")
	}
}

// TestManagerLoginSuccess 驗證正確密碼登入並產生工作階段。
func TestManagerLoginSuccess(t *testing.T) {
	m := newTestManager(t)
	id, sess, ok := m.Login("admin-pass-2026-strong")
	if !ok {
		t.Fatal("正確密碼應登入成功")
	}
	if id == "" {
		t.Fatal("session ID 不應為空")
	}
	if sess.CSRFToken == "" {
		t.Fatal("CSRF token 不應為空")
	}
	if !sess.ExpiresAt.Equal(fixedNow.Add(12 * time.Hour)) {
		t.Fatalf("過期時間錯誤: %v", sess.ExpiresAt)
	}
	// 兩個登入 token 不相同(即使使用確定性隨機源,ID 也應變化)
	id2, sess2, ok2 := m.Login("admin-pass-2026-strong")
	if !ok2 {
		t.Fatal("第二次登入應成功")
	}
	if id == id2 {
		t.Fatal("兩個登入的 session ID 不應相同")
	}
	if sess.CSRFToken == sess2.CSRFToken {
		t.Fatal("兩個登入的 CSRF 不應相同")
	}
}

// TestManagerValidate 驗證工作階段校驗與 CSRF。
func TestManagerValidate(t *testing.T) {
	m := newTestManager(t)
	id, sess, ok := m.Login("admin-pass-2026-strong")
	if !ok {
		t.Fatal("登入失敗")
	}
	if got, ok2 := m.Validate(id); !ok2 || got.CSRFToken != sess.CSRFToken {
		t.Fatalf("有效工作階段應通過校驗: %+v ok=%v", got, ok2)
	}
	if _, ok2 := m.Validate("nonexistent"); ok2 {
		t.Fatal("不存在的工作階段不應通過")
	}
	if !m.ValidateCSRF(id, sess.CSRFToken) {
		t.Fatal("正確 CSRF 應通過")
	}
	if m.ValidateCSRF(id, "wrong-token") {
		t.Fatal("錯誤 CSRF 不應通過")
	}
}

// TestManagerSessionExpiry 驗證工作階段過期後失效。
func TestManagerSessionExpiry(t *testing.T) {
	now := fixedNow
	opts := Options{
		Password: "admin-pass-2026-strong",
		TTL:      12 * time.Hour,
		Now:      func() time.Time { return now },
		Random:   &fixedRandom{},
	}
	m, err := NewManager(opts)
	if err != nil {
		t.Fatal(err)
	}
	id, _, ok := m.Login("admin-pass-2026-strong")
	if !ok {
		t.Fatal("登入失敗")
	}
	// 時間越過 TTL 後工作階段應失效
	now = now.Add(13 * time.Hour)
	if _, ok := m.Validate(id); ok {
		t.Fatal("過期工作階段不應通過校驗")
	}
}

// TestManagerLogout 驗證退出立即失效。
func TestManagerLogout(t *testing.T) {
	m := newTestManager(t)
	id, _, ok := m.Login("admin-pass-2026-strong")
	if !ok {
		t.Fatal("登入失敗")
	}
	m.Logout(id)
	if _, ok := m.Validate(id); ok {
		t.Fatal("退出後工作階段應失效")
	}
	if m.ValidateCSRF(id, "token") {
		t.Fatal("退出後 CSRF 不應通過")
	}
}

// TestManagerStoresHashedKeys 驗證 map 不以原始 session ID 為鍵。
func TestManagerStoresHashedKeys(t *testing.T) {
	m := newTestManager(t)
	id, _, ok := m.Login("admin-pass-2026-strong")
	if !ok {
		t.Fatal("登入失敗")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.sessions {
		keyStr := string(key[:])
		if strings.Contains(keyStr, id) {
			t.Fatalf("map key 洩露原始 session ID: %q", keyStr)
		}
		if keyStr == id {
			t.Fatal("map 直接以 session ID 為鍵")
		}
		if sha256.Sum256([]byte(id)) != key {
			t.Fatalf("map key 不是 session ID 的 SHA-256: %q", keyStr)
		}
	}
}

// TestManagerRejectsWeakPassword 驗證構造器拒絕短於 8 字元的密碼。
func TestManagerRejectsWeakPassword(t *testing.T) {
	opts := Options{
		Password: "short",
		TTL:      12 * time.Hour,
		Now:      func() time.Time { return fixedNow },
		Random:   &fixedRandom{},
	}
	if _, err := NewManager(opts); err == nil {
		t.Fatal("短密碼應被拒絕")
	}
	if _, err := NewManager(Options{}); err == nil {
		t.Fatal("空密碼應被拒絕")
	}
	// 7 字元仍拒絕
	opts.Password = "1234567"
	if _, err := NewManager(opts); err == nil {
		t.Fatal("7 字元密碼應被拒絕")
	}
	// 8 字元接受
	opts.Password = "12345678"
	if _, err := NewManager(opts); err != nil {
		t.Fatalf("8 字元密碼應被接受: %v", err)
	}
}

// TestManagerSessionLimit 驗證工作階段數量上限與淘汰。
func TestManagerSessionLimit(t *testing.T) {
	// 不同過期時間:模擬最早過期的工作階段被優先淘汰
	now := fixedNow
	opts := Options{
		Password: "admin-pass-2026-strong",
		TTL:      12 * time.Hour,
		Now:      func() time.Time { return now },
		Random:   &fixedRandom{},
	}
	m, err := NewManager(opts)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, maxSessions+5)
	for i := 0; i < maxSessions+5; i++ {
		id, _, ok := m.Login("admin-pass-2026-strong")
		if !ok {
			t.Fatalf("第 %d 次登入失敗", i)
		}
		ids = append(ids, id)
		now = now.Add(time.Minute)
	}
	m.mu.Lock()
	count := len(m.sessions)
	m.mu.Unlock()
	if count > maxSessions {
		t.Fatalf("工作階段數 %d 超過上限 %d", count, maxSessions)
	}
	// 最早的工作階段應被淘汰
	if _, ok := m.Validate(ids[0]); ok {
		t.Fatal("最早過期的工作階段應被淘汰")
	}
	// 最新的工作階段應仍有效
	if _, ok := m.Validate(ids[len(ids)-1]); !ok {
		t.Fatal("最新工作階段應仍有效")
	}
}

// TestManagerConstantTimeCompare 驗證 CSRF 使用常量時間比較(僅檢查行為正確)。
func TestManagerConstantTimeCompare(t *testing.T) {
	m := newTestManager(t)
	id, sess, ok := m.Login("admin-pass-2026-strong")
	if !ok {
		t.Fatal("登入失敗")
	}
	// 構造與正確 token 長度相同但內容不同的 token
	wrong := strings.Repeat("A", len(sess.CSRFToken))
	if m.ValidateCSRF(id, wrong) {
		t.Fatal("錯誤的 CSRF 不應通過")
	}
	if m.ValidateCSRF(id, sess.CSRFToken[:len(sess.CSRFToken)-1]+"B") {
		t.Fatal("尾字元不同的 CSRF 不應通過")
	}
}

// TestManagerBadReader 驗證隨機源失敗時構造器報錯。
func TestManagerBadReader(t *testing.T) {
	opts := Options{
		Password: "admin-pass-2026-strong",
		TTL:      12 * time.Hour,
		Now:      func() time.Time { return fixedNow },
		Random:   bytes.NewReader(nil), // EOF
	}
	if _, err := NewManager(opts); err == nil {
		t.Fatal("隨機源失敗時構造器應報錯")
	}
}
