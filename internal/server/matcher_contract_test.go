package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"icloud-hme/internal/account"
)

// TestClassifyLoginErrMatchesRealProducerLiteral 是「比對字串」與「產生字串」的
// 契約測試。
//
// classifyLoginErr 用 strings.Contains 比對 internal/hme/auth.go 產生的錯誤文字。
// 這種耦合沒有編譯期保護：只要有人改了產生端的一邊（例如把簡體翻成繁體），
// 比對就會靜默失效，OTP 錯誤會被誤判成一般上游故障。
// 因此這裡直接讀原始碼，確認比對條件確實出現在產生端檔案裡。
func TestClassifyLoginErrMatchesRealProducerLiteral(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "hme", "auth.go"))
	if err != nil {
		t.Fatalf("讀取 hme/auth.go 失敗: %v", err)
	}
	src := string(raw)

	cases := []struct {
		name       string
		producer   string // 產生端必須包含的字面值
		errText    string
		wantCode   string
		srcFile    string
		originNote string
	}{
		{
			name:       "需要提供 OTP",
			producer:   "需要提供 OTP",
			errText:    "帳號已啟用雙重認證，需要提供 OTP",
			wantCode:   "OTP_REQUIRED",
			srcFile:    "hme/auth.go",
			originNote: "hme/auth.go handleTwoFactor",
		},
		{
			name:       "2FA 驗證失敗",
			producer:   "2FA 驗證失敗",
			errText:    "2FA 驗證失敗：HTTP 401",
			wantCode:   "OTP_INVALID",
			srcFile:    "hme/auth.go",
			originNote: "hme/auth.go verify2FA",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(src, tc.producer) {
				t.Fatalf("比對條件 %q 已不存在於 %s：classifyLoginErr 的比對字串與產生端不同步",
					tc.producer, tc.srcFile)
			}
			be := classifyLoginErr(errors.New(tc.errText))
			if be.Code != tc.wantCode {
				t.Fatalf("期望 code=%q，得到 %q（比對字串與 %s 不同步）",
					tc.wantCode, be.Code, tc.originNote)
			}
		})
	}
}

// TestMapAccountErrMatchesRealProducerLiteral 確認「帳號尚未設定 Cookie」的比對條件
// 與 internal/account/manager.go 的實際輸出同步。
//
// 這裡直接呼叫真實的 Manager，而不是手寫字串，因此產生端一旦改字就會立刻失敗。
func TestMapAccountErrMatchesRealProducerLiteral(t *testing.T) {
	dir := t.TempDir()
	mgr, err := account.NewManager(dir)
	if err != nil {
		t.Fatalf("建立 Manager 失敗: %v", err)
	}
	sum, err := mgr.AddAccountWithInput(account.AddAccountInput{
		Name:        "主號",
		ICloudEmail: "owner@icloud.com",
	})
	if err != nil {
		t.Fatalf("新增帳號失敗: %v", err)
	}

	// 未設定 Cookie 的帳號在建立 HME 用戶端時必須產生「尚未設定 Cookie」錯誤。
	_, err = mgr.HMEClient(sum.ID, false)
	if err == nil {
		t.Fatal("未設定 Cookie 的帳號不應取得可用用戶端")
	}

	be := mapAccountErr(err)
	if be.Status != 400 || be.Code != "VALIDATION_ERROR" {
		t.Fatalf("期望 400/VALIDATION_ERROR，得到 %d/%s", be.Status, be.Code)
	}
	// 不得把內部錯誤原文直接外洩給前端。
	if be.Message != "帳號尚未設定 Cookie" {
		t.Fatalf("期望對外訊息為「帳號尚未設定 Cookie」，得到 %q（產生端字串=%q）",
			be.Message, err.Error())
	}
}
