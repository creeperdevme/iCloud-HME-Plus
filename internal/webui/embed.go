// Package webui 提供內嵌前端靜態資源服務與 SPA 路由 fallback。
//
// 生產產物寫入 internal/webui/dist/ 並由 embed.FS 內嵌,隨單個 Go 二進制分發。
package webui

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var embedded embed.FS

// dist 是內嵌的 dist 子檔案系統。
var dist fs.FS

func init() {
	dist, _ = fs.Sub(embedded, "dist")
}

// Embedded 返回內嵌的 dist 檔案系統。
func Embedded() (fs.FS, error) {
	if dist == nil {
		return nil, errors.New("前端資源尚未建置")
	}
	return dist, nil
}

// mimeByExt 返回常見擴展名對應的 Content-Type。
func mimeByExt(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".gif"):
		return "image/gif"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".txt"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// hashedAsset 判斷路徑是否含雜湊檔案名(如 app-abc123.js)。
func hashedAsset(name string) bool {
	base := path.Base(name)
	dot := strings.Index(base, ".")
	if dot < 0 {
		return false
	}
	return len(base[:dot]) > 1 && strings.Contains(base[:dot], "-")
}

// Handler 返回服務內嵌靜態資源並支援 SPA fallback 的 http.Handler。
//
// 只允許 GET/HEAD;路徑先 path.Clean 並拒絕 "..";存在檔案就按擴展名服務,
// 不存在且路徑不含檔案擴展名時返回 index.html;生產 dist 未產生時返回
// 清晰的 503 中文純文本,而不是 panic。
func Handler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "不允許的方法", http.StatusMethodNotAllowed)
			return
		}

		clean := path.Clean("/" + r.URL.Path)
		if strings.Contains(clean, "..") {
			http.Error(w, "路徑無效", http.StatusBadRequest)
			return
		}
		name := strings.TrimPrefix(clean, "/")
		if name == "" {
			name = "index.html"
		}

		// 檢查檔案是否存在
		file, err := fsys.Open(name)
		if err == nil {
			defer file.Close()
			if info, statErr := file.Stat(); statErr == nil && !info.IsDir() {
				if strings.HasSuffix(name, ".html") || name == "index.html" {
					w.Header().Set("Cache-Control", "no-cache")
				} else if hashedAsset(name) {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				w.Header().Set("Content-Type", mimeByExt(name))
				// 顯式寫狀態,避免繼承外層(如 gin NoRoute)預設的狀態碼
				w.WriteHeader(http.StatusOK)
				if r.Method == http.MethodHead {
					return
				}
				_, _ = io.Copy(w, file)
				return
			}
			// 目錄:落到 SPA fallback
		}

		// 路徑含檔案擴展名 → 404(不 fallback)
		if path.Ext(clean) != "" && clean != "/" && !strings.HasSuffix(clean, "/") {
			http.NotFound(w, r)
			return
		}

		// SPA fallback:返回 index.html(不存在時 503 中文提示)
		index, err := fsys.Open("index.html")
		if err != nil {
			msg := "管理介面資源尚未建置，請先執行 npm run build 後重新編譯服務"
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintln(w, msg)
			return
		}
		defer index.Close()
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// 顯式寫狀態,避免繼承外層(如 gin NoRoute)預設的狀態碼
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.Copy(w, index)
	})
}
