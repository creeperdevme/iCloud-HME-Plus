package webhook

import "testing"

func TestRenderTemplate(t *testing.T) {
	envelope := map[string]any{
		"event":     "email_received",
		"timestamp": "2026-01-01T00:00:00Z",
		"payload":   map[string]any{"subject": "驗證碼", "preview": "代碼 123456"},
	}
	got := renderTemplate(`{"event":{{event}},"subject":{{payload.subject}},"preview":{{payload.preview}}}`, envelope)
	want := `{"event":"email_received","subject":"驗證碼","preview":"代碼 123456"}`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
