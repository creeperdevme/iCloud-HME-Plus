package mail

import "testing"

func TestSanitizePreviewRemovesInvisibleHTMLBlocks(t *testing.T) {
	raw := `<html><head><style>@font-face { font-family: Söhne; } body { color: red; }</style></head><body><p>驗證碼：123456</p><script>alert(1)</script></body></html>`
	if got := sanitizePreview(raw); got != "驗證碼：123456" {
		t.Fatalf("sanitizePreview() = %q, want readable body", got)
	}
}

func TestSanitizePreviewDropsCSSOnlyContent(t *testing.T) {
	raw := `@font-face { font-family: Söhne; } .ExternalClass { line-height: 100%; } #bodyTable { width: 560px; } body { font-family: Helvetica, Arial, sans-serif; }`
	if got := sanitizePreview(raw); got != "" {
		t.Fatalf("sanitizePreview() = %q, want empty CSS preview", got)
	}
}

func TestSanitizePreviewRemovesCSSPrefixAndKeepsBody(t *testing.T) {
	raw := `@font-face { font-family: Söhne; } .ExternalClass { line-height: 100%; } 正文內容`
	if got := sanitizePreview(raw); got != "正文內容" {
		t.Fatalf("sanitizePreview() = %q, want body text", got)
	}
}

func TestSanitizePreviewKeepsNormalText(t *testing.T) {
	raw := `訂單號：A-123; 請在 10:00 前完成驗證。`
	if got := sanitizePreview(raw); got != raw {
		t.Fatalf("sanitizePreview() = %q, want %q", got, raw)
	}
}
