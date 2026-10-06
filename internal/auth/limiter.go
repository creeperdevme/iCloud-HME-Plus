package auth

import (
	"sync"
	"time"
)

// limiterEntry 記錄一個 key 的失敗次數與窗口起點。
type limiterEntry struct {
	firstFailure time.Time
	failures     int
}

// Limiter 按用戶端 IP 的登入失敗限流(固定窗口),執行緒安全。
//
// 每次訪問時刪除已過窗口的 key,key 數達到上限時淘汰最早窗口,
// 避免偽造來源位址造成記憶體 DoS。
type Limiter struct {
	mu          sync.Mutex
	now         func() time.Time
	window      time.Duration
	maxFailures int
	maxKeys     int
	failures    map[string]limiterEntry
}

// NewLimiter 建立限流器。now 為可注入時鐘,window 為固定窗口,
// maxFailures 為窗口內允許的最大失敗次數,maxKeys 為 key 數上限。
func NewLimiter(now func() time.Time, window time.Duration, maxFailures, maxKeys int) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		now:         now,
		window:      window,
		maxFailures: maxFailures,
		maxKeys:     maxKeys,
		failures:    make(map[string]limiterEntry),
	}
}

// Allow 記錄一次失敗並判斷是否允許繼續嘗試。
//
// 返回 allowed=false 時,retryAfter 為建議等待時間(不超過窗口)。
func (l *Limiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.failures[key]
	if !exists || now.Sub(entry.firstFailure) >= l.window {
		// 新窗口:重置計數
		l.failures[key] = limiterEntry{firstFailure: now, failures: 1}
		l.gcLocked(now)
		return true, 0
	}
	entry.failures++
	if entry.failures > l.maxFailures {
		l.failures[key] = entry
		retryAfter = l.window - now.Sub(entry.firstFailure)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}
	l.failures[key] = entry
	return true, 0
}

// Success 標記登入成功,清除該 key 的失敗記錄。
func (l *Limiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// gcLocked 清理過窗 key 並淘汰最早窗口,須持鎖呼叫。
func (l *Limiter) gcLocked(now time.Time) {
	for k, e := range l.failures {
		if now.Sub(e.firstFailure) >= l.window {
			delete(l.failures, k)
		}
	}
	if len(l.failures) > l.maxKeys {
		oldest := time.Time{}
		var oldestKey string
		for k, e := range l.failures {
			if oldest.IsZero() || e.firstFailure.Before(oldest) {
				oldest = e.firstFailure
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(l.failures, oldestKey)
		}
	}
}
