// Package tempmail 追蹤「隨機信箱」（臨時別名）的生命週期。
//
// 隨機信箱是建立在某個 iCloud 帳號下的 Hide My Email 別名，預設 24 小時後由
// 背景清理程式自動刪除；使用者也可以標記為「不自動刪除」，或立刻手動刪除。
//
// 本套件只負責追蹤與持久化，實際建立/刪除別名由呼叫端透過 Backend 執行。
package tempmail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileName 是持久化檔名。
const FileName = "temp_mailboxes.json"

// ErrNotFound 表示找不到指定的隨機信箱。
var ErrNotFound = errors.New("找不到這個隨機信箱")

// Mailbox 是一個被追蹤的隨機信箱。
type Mailbox struct {
	// ID 是別名的 anonymousId，刪除時使用。
	ID string `json:"id"`
	// Email 是實際生效的隨機信箱位址。
	Email string `json:"email"`
	// AccountID 是別名所屬的 iCloud 帳號。
	AccountID string `json:"account_id"`
	// Label 是建立時寫入的標籤。
	Label string `json:"label"`
	// CreatedAt 是建立時間。
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt 是自動刪除時間；Keep 為 true 時不具意義。
	ExpiresAt time.Time `json:"expires_at"`
	// Keep 為 true 表示不自動刪除。
	Keep bool `json:"keep"`
	// Attempts 記錄自動刪除連續失敗次數，避免上游持續失敗時無限重試。
	Attempts int `json:"attempts,omitempty"`
	// LastError 是最近一次自動刪除失敗的原因。
	LastError string `json:"last_error,omitempty"`
}

// Expired 回報在 now 時是否已到期需要刪除。
func (m Mailbox) Expired(now time.Time) bool {
	return !m.Keep && !m.ExpiresAt.IsZero() && !now.Before(m.ExpiresAt)
}

// Remaining 回報距離自動刪除還有多久；不自動刪除或已到期時為 0。
func (m Mailbox) Remaining(now time.Time) time.Duration {
	if m.Keep || m.ExpiresAt.IsZero() {
		return 0
	}
	if d := m.ExpiresAt.Sub(now); d > 0 {
		return d
	}
	return 0
}

// Store 是隨機信箱的記憶體索引加上 JSON 持久化。
//
// path 為空字串時只存在記憶體中（測試用）。
type Store struct {
	mu    sync.Mutex
	path  string
	items map[string]Mailbox
}

// NewStore 建立 Store 並載入既有資料。path 為空時不持久化。
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]Mailbox{}}
	if path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("讀取隨機信箱記錄失敗：%w", err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	var items []Mailbox
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("解析隨機信箱記錄失敗：%w", err)
	}
	for _, m := range items {
		if m.ID == "" || m.Email == "" {
			continue
		}
		s.items[m.ID] = m
	}
	return s, nil
}

// Add 新增一筆記錄。
func (s *Store) Add(m Mailbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[m.ID] = m
	return s.saveLocked()
}

// List 依建立時間由新到舊返回所有記錄。
func (s *Store) List() []Mailbox {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Mailbox, 0, len(s.items))
	for _, m := range s.items {
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// Get 取得單筆記錄。
func (s *Store) Get(id string) (Mailbox, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.items[id]
	return m, ok
}

// Remove 移除記錄（實際刪除別名由呼叫端負責）。
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return ErrNotFound
	}
	delete(s.items, id)
	return s.saveLocked()
}

// SetKeep 設定是否不自動刪除。
//
// keep=true 會清除到期時間；keep=false 會重新起算 ttl，避免使用者切換後
// 因為原本的到期時間已過而立刻被刪除。
func (s *Store) SetKeep(id string, keep bool, ttl time.Duration, now time.Time) (Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.items[id]
	if !ok {
		return Mailbox{}, ErrNotFound
	}
	m.Keep = keep
	m.Attempts = 0
	m.LastError = ""
	if keep {
		m.ExpiresAt = time.Time{}
	} else {
		m.ExpiresAt = now.Add(ttl)
	}
	s.items[id] = m
	if err := s.saveLocked(); err != nil {
		return Mailbox{}, err
	}
	return m, nil
}

// Due 返回在 now 時已到期且需要自動刪除的記錄。
func (s *Store) Due(now time.Time) []Mailbox {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Mailbox
	for _, m := range s.items {
		if m.Expired(now) {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	return out
}

// RecordFailure 記錄一次自動刪除失敗，並回傳累計次數。
func (s *Store) RecordFailure(id, reason string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.items[id]
	if !ok {
		return 0
	}
	m.Attempts++
	m.LastError = reason
	s.items[id] = m
	_ = s.saveLocked()
	return m.Attempts
}

// Len 回傳追蹤中的數量。
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	items := make([]Mailbox, 0, len(s.items))
	for _, m := range s.items {
		items = append(items, m)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化隨機信箱記錄失敗：%w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("建立資料目錄失敗：%w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("寫入隨機信箱記錄失敗：%w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("更新隨機信箱記錄失敗：%w", err)
	}
	return nil
}
