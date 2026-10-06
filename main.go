// Command icloud-hme 啟動 iCloud Hide My Email 多帳號管理平台。
//
// 兩個核心 HTTP 介面:
//
//	POST /api/create  — 建立隱私信箱別名
//	GET  /api/inbox   — 讀取郵件
//
// 用法:
//
//	./icloud-hme                    # 預設 :8081
//	./icloud-hme -addr :9000        # 指定連接埠
//	./icloud-hme -data ./data       # 指定資料目錄
//	./icloud-hme -debug             # 偵錯模式
//	./icloud-hme -log-level debug   # 日誌級別 (debug/info/warn/error)
//
// 安全配置(必填):
//
//	ICLOUD_HME_ADMIN_PASSWORD      管理員密碼,至少 8 字元(行程啟動後從環境清除)
//	ICLOUD_HME_SESSION_TTL         工作階段有效期,預設 12h,範圍 15m-168h
//	ICLOUD_HME_SECURE_COOKIE       TLS 反向代理部署時設為 true
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"icloud-hme/internal/account"
	"icloud-hme/internal/server"
)

func main() {
	addr := flag.String("addr", ":8081", "HTTP 監聽位址")
	dataDir := flag.String("data", "./data", "資料目錄（accounts.json 存放位置）")
	debug := flag.Bool("debug", false, "偵錯模式（啟用 Gin 偵錯日誌）")
	flag.Parse()

	adminPassword := os.Getenv("ICLOUD_HME_ADMIN_PASSWORD")
	if len(adminPassword) < 8 {
		log.Fatal("請透過環境變數 ICLOUD_HME_ADMIN_PASSWORD 設定管理員密碼（至少 8 個字元）")
	}
	sessionTTL, err := parseSessionTTL(os.Getenv("ICLOUD_HME_SESSION_TTL"))
	if err != nil {
		log.Fatalf("ICLOUD_HME_SESSION_TTL 無效：%v", err)
	}
	secureCookie := os.Getenv("ICLOUD_HME_SECURE_COOKIE") == "true"

	log.Printf("iCloud Hide My Email 服務啟動 addr=%s", *addr)

	abs, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatalf("資料目錄路徑錯誤：%v", err)
	}

	mgr, err := account.NewManager(abs)
	if err != nil {
		log.Fatalf("初始化帳號管理器失敗：%v", err)
	}
	defer mgr.Close()
	count := len(mgr.ListAccounts())
	log.Printf("帳號載入完成 count=%d data_dir=%s", count, abs)

	srv, err := server.New(mgr, server.Config{
		Debug:         *debug,
		AdminPassword: adminPassword,
		SessionTTL:    sessionTTL,
		SecureCookie:  secureCookie,
	})
	if err != nil {
		log.Fatalf("初始化服務失敗：%v", err)
	}

	// 密碼只用於初始化認證,隨後立即從行程環境清除
	_ = os.Unsetenv("ICLOUD_HME_ADMIN_PASSWORD")

	log.Printf("HTTP 服務就緒 addr=%s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服務啟動失敗：%v", err)
	}
}

// parseSessionTTL 解析工作階段有效期,預設 12h,範圍 15m-168h。
func parseSessionTTL(raw string) (time.Duration, error) {
	if raw == "" {
		return 12 * time.Hour, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 15*time.Minute || d > 168*time.Hour {
		return 0, err
	}
	return d, nil
}
