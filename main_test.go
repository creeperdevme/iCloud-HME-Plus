package main

import (
	"testing"
	"time"
)

func TestTempTTLFromEnv(t *testing.T) {
	// 未設定時用預設 24h。
	t.Setenv("ICLOUD_HME_TEMP_TTL", "")
	got, err := tempTTLFromEnv()
	if err != nil {
		t.Fatalf("未設定時不應報錯：%v", err)
	}
	if got != defaultTempTTL {
		t.Errorf("未設定時 = %v，期望 %v", got, defaultTempTTL)
	}

	// 這個案例會抓到「把變數名稱當成值傳進去」的錯誤：那樣就完全讀不到環境變數。
	t.Setenv("ICLOUD_HME_TEMP_TTL", "6h")
	got, err = tempTTLFromEnv()
	if err != nil {
		t.Fatalf("6h 應可解析：%v", err)
	}
	if got != 6*time.Hour {
		t.Errorf("設定 6h 時 = %v，期望 6h（環境變數沒有被正確讀取）", got)
	}

	// 邊界值。
	for _, raw := range []string{"1m", "720h"} {
		t.Setenv("ICLOUD_HME_TEMP_TTL", raw)
		if _, err := tempTTLFromEnv(); err != nil {
			t.Errorf("%s 應該合法：%v", raw, err)
		}
	}
}

func TestTempTTLFromEnvRejectsInvalid(t *testing.T) {
	for _, raw := range []string{"0s", "30s", "721h", "abc", "24", "-1h"} {
		t.Setenv("ICLOUD_HME_TEMP_TTL", raw)
		if _, err := tempTTLFromEnv(); err == nil {
			t.Errorf("%q 應該被拒絕", raw)
		}
	}
}

func TestParseSessionTTL(t *testing.T) {
	if got, err := parseSessionTTL(""); err != nil || got != 12*time.Hour {
		t.Errorf("未設定時 = %v, %v，期望 12h", got, err)
	}
	if got, err := parseSessionTTL("30m"); err != nil || got != 30*time.Minute {
		t.Errorf("30m = %v, %v", got, err)
	}
	// 超出範圍必須回傳錯誤（早期版本會回傳 nil 錯誤搭配 0 值）。
	for _, raw := range []string{"10m", "169h", "garbage"} {
		if _, err := parseSessionTTL(raw); err == nil {
			t.Errorf("%q 應該被拒絕", raw)
		}
	}
}

func TestParseDurationEnv(t *testing.T) {
	fallback := 5 * time.Hour
	got, err := parseDurationEnv("", fallback, time.Minute, time.Hour*10)
	if err != nil || got != fallback {
		t.Errorf("空值 = %v, %v，期望 fallback", got, err)
	}
	if _, err := parseDurationEnv("1h", fallback, 2*time.Hour, 10*time.Hour); err == nil {
		t.Error("低於下限應該被拒絕")
	}
	if _, err := parseDurationEnv("11h", fallback, time.Hour, 10*time.Hour); err == nil {
		t.Error("高於上限應該被拒絕")
	}
}
