package auth

import (
	"testing"
	"time"
)

// TestLimiterAllowsUpToMax 驗證固定窗口內最多允許 maxFailures 次失敗。
func TestLimiterAllowsUpToMax(t *testing.T) {
	now := fixedNow
	l := NewLimiter(func() time.Time { return now }, 15*time.Minute, 5, 10000)
	key := "203.0.113.10"
	for i := 1; i <= 5; i++ {
		allowed, _ := l.Allow(key)
		if !allowed {
			t.Fatalf("第 %d 次失敗應允許", i)
		}
	}
	// 第 6 次應被拒絕,retryAfter 不超過窗口
	allowed, retryAfter := l.Allow(key)
	if allowed {
		t.Fatal("第 6 次失敗應被限流")
	}
	if retryAfter <= 0 || retryAfter > 15*time.Minute {
		t.Fatalf("retryAfter 應在 (0, 15m] 內,得到 %v", retryAfter)
	}
}

// TestLimiterWindowReset 驗證時鐘越過窗口後恢復。
func TestLimiterWindowReset(t *testing.T) {
	now := fixedNow
	l := NewLimiter(func() time.Time { return now }, 15*time.Minute, 5, 10000)
	key := "203.0.113.11"
	for i := 1; i <= 5; i++ {
		l.Allow(key)
	}
	if allowed, _ := l.Allow(key); allowed {
		t.Fatal("窗口內應被限流")
	}
	// 越過窗口
	now = now.Add(16 * time.Minute)
	if allowed, _ := l.Allow(key); !allowed {
		t.Fatal("越過窗口後應恢復")
	}
}

// TestLimiterSuccessResets 驗證呼叫 Success 後恢復。
func TestLimiterSuccessResets(t *testing.T) {
	now := fixedNow
	l := NewLimiter(func() time.Time { return now }, 15*time.Minute, 5, 10000)
	key := "203.0.113.12"
	for i := 1; i <= 5; i++ {
		l.Allow(key)
	}
	if allowed, _ := l.Allow(key); allowed {
		t.Fatal("失敗達到上限應被限流")
	}
	l.Success(key)
	if allowed, _ := l.Allow(key); !allowed {
		t.Fatal("Success 後應恢復")
	}
}

// TestLimiterIndependentKeys 驗證不同 key 互不影響。
func TestLimiterIndependentKeys(t *testing.T) {
	now := fixedNow
	l := NewLimiter(func() time.Time { return now }, 15*time.Minute, 5, 10000)
	for i := 1; i <= 5; i++ {
		l.Allow("203.0.113.20")
	}
	if allowed, _ := l.Allow("203.0.113.20"); allowed {
		t.Fatal("key A 應被限流")
	}
	if allowed, _ := l.Allow("203.0.113.21"); !allowed {
		t.Fatal("key B 不應受 key A 影響")
	}
}

// TestLimiterMaxKeys 驗證達到 key 上限時淘汰最早窗口且不 panic。
func TestLimiterMaxKeys(t *testing.T) {
	now := fixedNow
	l := NewLimiter(func() time.Time { return now }, 15*time.Minute, 5, 10)
	keys := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		key := "host-" + string(rune('a'+i))
		keys = append(keys, key)
		l.Allow(key)
		now = now.Add(time.Minute)
	}
	l.mu.Lock()
	count := len(l.failures)
	l.mu.Unlock()
	if count > 10 {
		t.Fatalf("key 數 %d 超過上限 10", count)
	}
	// 最早的 key 應被淘汰,新 key 可用
	if allowed, _ := l.Allow("fresh-host"); !allowed {
		t.Fatal("新 key 應可用")
	}
}
