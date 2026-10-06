// Package auth 實現管理員密碼校驗、記憶體工作階段、CSRF 與登入限流。
//
// 不依賴 Gin:HTTP 中介軟體只負責 Cookie/Header 與 HTTP 狀態映射。
// 工作階段只存記憶體,行程重啟即失效;session ID 與 CSRF 永不落盤或寫日誌。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	// DefaultTTL 是預設工作階段有效期。
	DefaultTTL = 12 * time.Hour
	// maxSessions 是同時有效工作階段上限,超出時淘汰最早過期的工作階段。
	maxSessions = 32
)

// Options 是 Manager 的構造選項。
type Options struct {
	Password string
	TTL      time.Duration
	Now      func() time.Time
	Random   io.Reader
}

// Session 是一次管理員工作階段的公開資訊。
type Session struct {
	CSRFToken string
	ExpiresAt time.Time
}

// sessionRecord 是記憶體工作階段記錄,map key 為 session ID 的 SHA-256。
type sessionRecord struct {
	csrfHash  [32]byte
	csrfToken string
	expiresAt time.Time
}

// Manager 管理管理員工作階段,執行緒安全。
type Manager struct {
	mu       sync.Mutex
	salt     []byte
	password []byte
	ttl      time.Duration
	now      func() time.Time
	random   io.Reader
	sessions map[[32]byte]sessionRecord
}

// NewManager 建立工作階段管理器。
//
// 拒絕空密碼與短於 8 字元的密碼;啟動時用隨機 salt + argon2.IDKey
// 派生密碼,登入時使用常量時間比較。
func NewManager(opts Options) (*Manager, error) {
	if len(opts.Password) < 8 {
		return nil, errors.New("管理員密碼長度不能少於 8 個字元")
	}
	if opts.TTL <= 0 {
		opts.TTL = DefaultTTL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Random == nil {
		opts.Random = rand.Reader
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(opts.Random, salt); err != nil {
		return nil, fmt.Errorf("產生密碼 salt 失敗：%w", err)
	}
	// argon2id: time=1, memory=64*1024 KiB, threads=4, keyLen=32
	derived := argon2.IDKey([]byte(opts.Password), salt, 1, 64*1024, 4, 32)

	return &Manager{
		salt:     salt,
		password: derived,
		ttl:      opts.TTL,
		now:      opts.Now,
		random:   opts.Random,
		sessions: make(map[[32]byte]sessionRecord),
	}, nil
}

// Login 校驗管理員密碼;成功時建立工作階段並返回 session ID。
func (m *Manager) Login(password string) (sessionID string, session Session, ok bool) {
	derived := argon2.IDKey([]byte(password), m.salt, 1, 64*1024, 4, 32)
	if subtle.ConstantTimeCompare(derived, m.password) != 1 {
		return "", Session{}, false
	}

	idBytes := make([]byte, 32)
	if _, err := io.ReadFull(m.random, idBytes); err != nil {
		return "", Session{}, false
	}
	csrf := make([]byte, 32)
	if _, err := io.ReadFull(m.random, csrf); err != nil {
		return "", Session{}, false
	}

	sessionID = base64.RawURLEncoding.EncodeToString(idBytes)
	token := base64.RawURLEncoding.EncodeToString(csrf)
	expiresAt := m.now().Add(m.ttl)

	key := sha256.Sum256([]byte(sessionID))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	// 工作階段數達到上限時淘汰最早過期的工作階段,防止記憶體無界增長
	if len(m.sessions) >= maxSessions {
		m.evictOldestLocked()
	}
	if _, exists := m.sessions[key]; exists {
		// 碰撞概率可忽略;保守起見拒絕本次登入
		return "", Session{}, false
	}
	m.sessions[key] = sessionRecord{
		csrfHash:  sha256.Sum256([]byte(token)),
		csrfToken: token,
		expiresAt: expiresAt,
	}

	return sessionID, Session{CSRFToken: token, ExpiresAt: expiresAt}, true
}

// Validate 校驗工作階段是否有效;每次呼叫先清理過期工作階段。
func (m *Manager) Validate(sessionID string) (Session, bool) {
	key := sha256.Sum256([]byte(sessionID))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	rec, exists := m.sessions[key]
	if !exists {
		return Session{}, false
	}
	return Session{CSRFToken: rec.csrfToken, ExpiresAt: rec.expiresAt}, true
}

// ValidateCSRF 校驗 CSRF token(常量時間比較)。
func (m *Manager) ValidateCSRF(sessionID, token string) bool {
	key := sha256.Sum256([]byte(sessionID))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	rec, exists := m.sessions[key]
	if !exists {
		return false
	}
	want := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(rec.csrfHash[:], want[:]) == 1
}

// Logout 刪除工作階段。
func (m *Manager) Logout(sessionID string) {
	key := sha256.Sum256([]byte(sessionID))
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, key)
}

// pruneLocked 清理所有過期工作階段,須持鎖呼叫。
func (m *Manager) pruneLocked() {
	now := m.now()
	for key, rec := range m.sessions {
		if !now.Before(rec.expiresAt) {
			delete(m.sessions, key)
		}
	}
}

// evictOldestLocked 淘汰最早過期的工作階段,須持鎖呼叫。
func (m *Manager) evictOldestLocked() {
	oldest := time.Time{}
	var oldestKey [32]byte
	for key, rec := range m.sessions {
		if oldest.IsZero() || rec.expiresAt.Before(oldest) {
			oldest = rec.expiresAt
			oldestKey = key
		}
	}
	if !oldest.IsZero() {
		delete(m.sessions, oldestKey)
	}
}
