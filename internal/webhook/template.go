package webhook

import (
	"encoding/json"
	"strings"
)

// renderTemplate replaces the supported webhook placeholders with JSON-safe values.
// Supported names: event, timestamp, payload, and payload.field (one level).
func renderTemplate(template string, envelope map[string]any) string {
	out := template
	replace := func(name string, value any) {
		raw, _ := json.Marshal(value)
		out = strings.ReplaceAll(out, "{{"+name+"}}", string(raw))
	}
	replace("event", envelope["event"])
	replace("timestamp", envelope["timestamp"])
	payload, _ := envelope["payload"].(map[string]any)
	replace("payload", payload)
	for key, value := range payload {
		replace("payload."+key, value)
	}
	return out
}
