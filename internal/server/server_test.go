package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"testing/fstest"
)

// TestSPAAPI404JSON 驗證 /api/not-found 返回 JSON 404 而非 HTML。
func TestSPAAPI404JSON(t *testing.T) {
	f := &fakeBackend{}
	s := newWithBackend(f, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	// 替換 webui 為記憶體 FS,避免依賴構建產物
	webuiFS = fstest.MapFS{
		"index.html": {Data: []byte("<html>管理介面</html>")},
	}
	defer func() { webuiFS = nil }()

	req := httptest.NewRequest("GET", "/api/not-found", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404,得到 %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Body.String(), `{"success":false`) {
		t.Fatalf("API 404 應返回 JSON: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("API 404 不應返回 HTML: %s", rec.Body.String())
	}
}

// TestSPAIndexServed 驗證根路徑由 webui 服務(返回 index 而非 404)。
func TestSPAIndexServed(t *testing.T) {
	f := &fakeBackend{}
	s := newWithBackend(f, Config{
		Debug:         false,
		AdminPassword: "admin-pass-2026-strong",
	})
	webuiFS = fstest.MapFS{
		"index.html": {Data: []byte("<html>管理介面</html>")},
	}
	defer func() { webuiFS = nil }()

	req := httptest.NewRequest("GET", "/accounts?account_id=acc_test", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA 路徑期望 200,得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "管理介面") {
		t.Fatalf("SPA 路徑應返回 index 內容: %s", rec.Body.String())
	}
}
