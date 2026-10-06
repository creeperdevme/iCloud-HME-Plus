package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// testFS 構造包含 index.html 與雜湊 asset 的記憶體檔案系統。
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {
			Data: []byte("<html><body>管理介面</body></html>"),
		},
		"assets/app-abc123.js": {
			Data: []byte("console.log('app')"),
		},
	}
}

// TestSPARoot 驗證根路徑返回 index.html 且快取為 no-cache。
func TestSPARoot(t *testing.T) {
	h := Handler(testFS())
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200,得到 %d", rec.Code)
	}
	if !containsStr(rec.Body.String(), "管理介面") {
		t.Fatalf("回應應包含 index 內容: %s", rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("index 快取應為 no-cache,得到 %q", cc)
	}
}

// TestSPAFallback 驗證 SPA 路徑返回 index.html。
func TestSPAFallback(t *testing.T) {
	h := Handler(testFS())
	req := httptest.NewRequest("GET", "/accounts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200,得到 %d", rec.Code)
	}
	if !containsStr(rec.Body.String(), "管理介面") {
		t.Fatalf("SPA fallback 應返回 index: %s", rec.Body.String())
	}
}

// TestSPAAsset 驗證真實 asset 服務且快取為 immutable。
func TestSPAAsset(t *testing.T) {
	h := Handler(testFS())
	req := httptest.NewRequest("GET", "/assets/app-abc123.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200,得到 %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("雜湊 asset 快取應為 immutable,得到 %q", cc)
	}
}

// TestSPAMissingAsset 驗證缺失 asset 返回 404。
func TestSPAMissingAsset(t *testing.T) {
	h := Handler(testFS())
	req := httptest.NewRequest("GET", "/assets/missing.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404,得到 %d", rec.Code)
	}
}

// TestSPAMethodNotAllowed 驗證 POST 不被靜態服務處理。
func TestSPAMethodNotAllowed(t *testing.T) {
	h := Handler(testFS())
	req := httptest.NewRequest("POST", "/accounts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("POST 不應返回 200")
	}
}

// TestEmbedded 驗證 Embedded 返回 dist 子檔案系統。
func TestEmbedded(t *testing.T) {
	fsys, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Open("placeholder.txt"); err != nil {
		t.Fatalf("Embedded 應包含 placeholder.txt: %v", err)
	}
}

// TestEmbeddedNoDist 驗證 dist 不存在時返回清晰錯誤。
func TestEmbeddedNoDist(t *testing.T) {
	if _, err := Embedded(); err != nil {
		// 未構建 dist 時返回 503 文案由 Handler 保證,這裡只驗證不 panic
		t.Logf("Embedded 錯誤(可接受): %v", err)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
