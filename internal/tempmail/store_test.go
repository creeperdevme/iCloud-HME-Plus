package tempmail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("測試時間解析失敗 %q：%v", s, err)
	}
	return v
}

func TestStoreAddAndListNewestFirst(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore：%v", err)
	}
	base := at(t, "2026-01-01T00:00:00Z")
	for _, m := range []Mailbox{
		{ID: "a", Email: "a@icloud.com", CreatedAt: base},
		{ID: "c", Email: "c@icloud.com", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "b", Email: "b@icloud.com", CreatedAt: base.Add(time.Hour)},
	} {
		if err := s.Add(m); err != nil {
			t.Fatalf("Add(%s)：%v", m.ID, err)
		}
	}
	got := s.List()
	if len(got) != 3 {
		t.Fatalf("List 長度 = %d，期望 3", len(got))
	}
	for i, want := range []string{"c", "b", "a"} {
		if got[i].ID != want {
			t.Errorf("List[%d] = %s，期望 %s（應由新到舊）", i, got[i].ID, want)
		}
	}
}

func TestStoreGetAndRemove(t *testing.T) {
	s, _ := NewStore("")
	if err := s.Add(Mailbox{ID: "x", Email: "x@icloud.com"}); err != nil {
		t.Fatalf("Add：%v", err)
	}
	if m, found := s.Get("x"); !found || m.Email != "x@icloud.com" {
		t.Fatalf("Get 回傳 %+v found=%v", m, found)
	}
	if _, found := s.Get("nope"); found {
		t.Error("Get 對不存在的 ID 應回 found=false")
	}
	if err := s.Remove("x"); err != nil {
		t.Fatalf("Remove：%v", err)
	}
	if err := s.Remove("x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("重複 Remove 應回 ErrNotFound，得到 %v", err)
	}
}

func TestMailboxExpiredIgnoresKeep(t *testing.T) {
	now := at(t, "2026-01-02T00:00:00Z")
	cases := []struct {
		name string
		mb   Mailbox
		want bool
	}{
		{"已到期", Mailbox{ExpiresAt: now.Add(-time.Minute)}, true},
		{"尚未到期", Mailbox{ExpiresAt: now.Add(time.Minute)}, false},
		{"不自動刪除即使時間已過", Mailbox{Keep: true, ExpiresAt: now.Add(-time.Hour)}, false},
		{"沒有到期時間", Mailbox{}, false},
	}
	for _, tc := range cases {
		if got := tc.mb.Expired(now); got != tc.want {
			t.Errorf("%s：Expired = %v，期望 %v", tc.name, got, tc.want)
		}
	}
}

func TestMailboxRemaining(t *testing.T) {
	now := at(t, "2026-01-02T00:00:00Z")
	if got := (Mailbox{ExpiresAt: now.Add(90 * time.Minute)}).Remaining(now); got != 90*time.Minute {
		t.Errorf("Remaining = %v，期望 1h30m", got)
	}
	if got := (Mailbox{Keep: true, ExpiresAt: now.Add(time.Hour)}).Remaining(now); got != 0 {
		t.Errorf("不自動刪除時 Remaining = %v，期望 0", got)
	}
	if got := (Mailbox{ExpiresAt: now.Add(-time.Hour)}).Remaining(now); got != 0 {
		t.Errorf("已到期時 Remaining = %v，期望 0（不應為負）", got)
	}
}

// TestStoreSetKeepRestartsCountdown 確認關閉「不自動刪除」時會重新起算，
// 而不是沿用已經過期的舊到期時間（否則使用者一關掉就被立刻刪除）。
func TestStoreSetKeepRestartsCountdown(t *testing.T) {
	s, _ := NewStore("")
	now := at(t, "2026-01-02T00:00:00Z")
	ttl := 24 * time.Hour
	// 建立一個已經過期的信箱，並標記為不自動刪除。
	if err := s.Add(Mailbox{ID: "x", Email: "x@icloud.com", ExpiresAt: now.Add(-time.Hour), Keep: true}); err != nil {
		t.Fatalf("Add：%v", err)
	}

	got, err := s.SetKeep("x", false, ttl, now)
	if err != nil {
		t.Fatalf("SetKeep：%v", err)
	}
	if got.Keep {
		t.Error("Keep 應為 false")
	}
	if want := now.Add(ttl); !got.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v，期望 %v（應從現在重新起算）", got.ExpiresAt, want)
	}
	if got.Expired(now) {
		t.Error("重新起算後不應立刻到期")
	}

	// 切回不自動刪除時要清掉到期時間。
	got, err = s.SetKeep("x", true, ttl, now)
	if err != nil {
		t.Fatalf("SetKeep(true)：%v", err)
	}
	if !got.ExpiresAt.IsZero() {
		t.Errorf("Keep=true 時 ExpiresAt 應為零值，得到 %v", got.ExpiresAt)
	}
	if got.Expired(now) {
		t.Error("Keep=true 不應到期")
	}

	if _, err := s.SetKeep("missing", true, ttl, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("對不存在的 ID SetKeep 應回 ErrNotFound，得到 %v", err)
	}
}

func TestStoreDue(t *testing.T) {
	s, _ := NewStore("")
	now := at(t, "2026-01-02T00:00:00Z")
	items := []Mailbox{
		{ID: "due-soon", Email: "a@icloud.com", ExpiresAt: now.Add(-2 * time.Minute)},
		{ID: "due-late", Email: "b@icloud.com", ExpiresAt: now.Add(-time.Minute)},
		{ID: "not-due", Email: "c@icloud.com", ExpiresAt: now.Add(time.Minute)},
		{ID: "kept", Email: "d@icloud.com", Keep: true},
	}
	for _, m := range items {
		if err := s.Add(m); err != nil {
			t.Fatalf("Add：%v", err)
		}
	}
	due := s.Due(now)
	if len(due) != 2 {
		t.Fatalf("Due 長度 = %d，期望 2（%+v）", len(due), due)
	}
	if due[0].ID != "due-soon" || due[1].ID != "due-late" {
		t.Errorf("Due 應依到期時間由早到晚排序，得到 %s, %s", due[0].ID, due[1].ID)
	}
	// 到期時間剛好等於 now 時視為到期。
	if err := s.Add(Mailbox{ID: "exact", Email: "e@icloud.com", ExpiresAt: now}); err != nil {
		t.Fatalf("Add：%v", err)
	}
	if got := len(s.Due(now)); got != 3 {
		t.Errorf("到期時間等於 now 時應算到期，Due 長度 = %d，期望 3", got)
	}
}

func TestStoreRecordFailure(t *testing.T) {
	s, _ := NewStore("")
	if err := s.Add(Mailbox{ID: "x", Email: "x@icloud.com"}); err != nil {
		t.Fatalf("Add：%v", err)
	}
	if n := s.RecordFailure("x", "上游錯誤"); n != 1 {
		t.Errorf("第一次失敗回傳 %d，期望 1", n)
	}
	if n := s.RecordFailure("x", "上游錯誤"); n != 2 {
		t.Errorf("第二次失敗回傳 %d，期望 2", n)
	}
	m, _ := s.Get("x")
	if m.Attempts != 2 || m.LastError != "上游錯誤" {
		t.Errorf("記錄未更新：%+v", m)
	}
	if n := s.RecordFailure("missing", "x"); n != 0 {
		t.Errorf("對不存在的 ID 應回 0，得到 %d", n)
	}
}

// TestStorePersistsAcrossReload 確認重啟後追蹤清單还在，
// 否則每次重啟都會讓已建立的隨機信箱永遠不會被自動刪除。
func TestStorePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore：%v", err)
	}
	base := at(t, "2026-01-01T00:00:00Z")
	if err := s.Add(Mailbox{
		ID: "keep-me", Email: "a@icloud.com", AccountID: "acc_1",
		Label: "temp-a-b-1", CreatedAt: base, ExpiresAt: base.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("Add：%v", err)
	}
	if err := s.Add(Mailbox{ID: "kept", Email: "b@icloud.com", Keep: true, CreatedAt: base}); err != nil {
		t.Fatalf("Add：%v", err)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("重新載入：%v", err)
	}
	if reloaded.Len() != 2 {
		t.Fatalf("重新載入後數量 = %d，期望 2", reloaded.Len())
	}
	m, found := reloaded.Get("keep-me")
	if !found {
		t.Fatal("重新載入後找不到 keep-me")
	}
	if m.Email != "a@icloud.com" || m.AccountID != "acc_1" || m.Label != "temp-a-b-1" {
		t.Errorf("欄位未正確還原：%+v", m)
	}
	if !m.CreatedAt.Equal(base) || !m.ExpiresAt.Equal(base.Add(24*time.Hour)) {
		t.Errorf("時間未正確還原：%+v", m)
	}
	kept, _ := reloaded.Get("kept")
	if !kept.Keep {
		t.Error("Keep 旗標未正確還原")
	}
	if !kept.ExpiresAt.IsZero() {
		t.Errorf("Keep 記錄的 ExpiresAt 應為零值，得到 %v", kept.ExpiresAt)
	}
}

func TestNewStoreMissingFileIsEmpty(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "nested", FileName))
	if err != nil {
		t.Fatalf("檔案不存在時不應報錯：%v", err)
	}
	if s.Len() != 0 {
		t.Errorf("應該是空的，得到 %d 筆", s.Len())
	}
}

func TestNewStoreRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("{ 這不是 JSON"), 0o600); err != nil {
		t.Fatalf("準備測試檔失敗：%v", err)
	}
	if _, err := NewStore(path); err == nil {
		t.Fatal("損毀的記錄檔應該回傳錯誤，讓呼叫端能決定要不要降級")
	}
}

// TestNewStoreSkipsIncompleteRecords 確認缺少關鍵欄位的記錄會被略過，
// 而不是產生一個永遠刪不掉的空殼。
func TestNewStoreSkipsIncompleteRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	raw := `[
		{"id":"ok","email":"a@icloud.com"},
		{"id":"","email":"b@icloud.com"},
		{"id":"no-email","email":""}
	]`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("準備測試檔失敗：%v", err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore：%v", err)
	}
	if s.Len() != 1 {
		t.Fatalf("數量 = %d，期望 1（只保留完整記錄）", s.Len())
	}
	if _, found := s.Get("ok"); !found {
		t.Error("完整記錄應該被保留")
	}
}
