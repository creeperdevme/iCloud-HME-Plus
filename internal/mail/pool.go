// IMAP 連線池: 按 Apple ID 複用長連線, 避免每次讀信都 TLS+Login。
package mail

import (
	"fmt"
	"sync"
	"time"
)

// Pool 管理按帳號複用的 IMAP 長連線。同一帳號循序使用(go-imap 非並行安全)。
type Pool struct {
	mu    sync.Mutex
	items map[string]*pooledConn
	// idleClose 空閒超過該時間則下次使用前重建; 0 表示不主動關。
	idleClose time.Duration
}

type pooledConn struct {
	mu          sync.Mutex
	appleID     string
	appPassword string
	client      *Client
	lastUsed    time.Time
}

// NewPool 建立連線池。
func NewPool() *Pool {
	return &Pool{
		items:     make(map[string]*pooledConn),
		idleClose: 10 * time.Minute,
	}
}

// Do 借出已連線的 Client 執行 fn; 用完不 Logout, 連線留在池中。
func (p *Pool) Do(appleID, appPassword string, fn func(*Client) error) error {
	if appleID == "" || appPassword == "" {
		return fmt.Errorf("IMAP 憑證為空")
	}
	pc := p.getOrCreate(appleID, appPassword)
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := pc.ensure(p.idleClose); err != nil {
		return err
	}
	err := fn(pc.client)
	pc.lastUsed = time.Now()
	if err != nil && isLikelyConnErr(err) {
		// 連線壞了, 丟掉, 下次重建
		pc.client.forceClose()
		pc.client = nil
	}
	return err
}

// Close 關閉池內全部連線。
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, pc := range p.items {
		pc.mu.Lock()
		if pc.client != nil {
			pc.client.Disconnect()
			pc.client = nil
		}
		pc.mu.Unlock()
		delete(p.items, k)
	}
}

func (p *Pool) getOrCreate(appleID, appPassword string) *pooledConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := appleID
	if pc, ok := p.items[key]; ok {
		// 密碼變更則換新
		if pc.appPassword != appPassword {
			pc.mu.Lock()
			if pc.client != nil {
				pc.client.forceClose()
				pc.client = nil
			}
			pc.appPassword = appPassword
			pc.mu.Unlock()
		}
		return pc
	}
	pc := &pooledConn{appleID: appleID, appPassword: appPassword}
	p.items[key] = pc
	return pc
}

func (pc *pooledConn) ensure(idleClose time.Duration) error {
	if pc.client != nil {
		// 空閒太久主動重建, 避免服務端靜默斷連
		if idleClose > 0 && !pc.lastUsed.IsZero() && time.Since(pc.lastUsed) > idleClose {
			pc.client.forceClose()
			pc.client = nil
		}
	}
	if pc.client != nil {
		if err := pc.client.Ping(); err == nil {
			return nil
		}
		pc.client.forceClose()
		pc.client = nil
	}
	c := NewClient(pc.appleID, pc.appPassword)
	if err := c.Connect(); err != nil {
		return err
	}
	pc.client = c
	pc.lastUsed = time.Now()
	return nil
}

func isLikelyConnErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// 常見斷連/IO 錯誤關鍵字
	for _, k := range []string{
		"connection reset", "broken pipe", "EOF", "i/o timeout",
		"use of closed", "not connected", "connection refused",
		"IMAP 連線", "wsarecv", "wsasend",
	} {
		if containsFold(s, k) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(sub) == 0 ||
		indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	// 小寫 ASCII 子串查找, 夠用
	sl := toLowerASCII(s)
	subl := toLowerASCII(sub)
	for i := 0; i+len(subl) <= len(sl); i++ {
		if sl[i:i+len(subl)] == subl {
			return i
		}
	}
	return -1
}

func toLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
