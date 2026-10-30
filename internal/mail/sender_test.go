package mail

import "testing"

func TestDecodeRelaySender(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"subdomain", "noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@icloud.com", "noreply@tm.openai.com"},
		{"mixed case local", "no-reply-mdPUDTvILQoXIafocg23Zg_at_mail_anthropic_com_4y4w6cpt990858_b416bc66@icloud.com", "no-reply-mdPUDTvILQoXIafocg23Zg@mail.anthropic.com"},
		{"short domain", "info_at_x_com_c0w3bbrdmfw0h8_2dqb9014@icloud.com", "info@x.com"},
		{"underscore in local", "no_reply_at_github_com_c0w3bbrdmfw0h8_2dqb9014@icloud.com", "no_reply@github.com"},
		{"at tld", "info_at_example_at_c0w3bbrdmfw0h8_2dqb9014@icloud.com", "info@example.at"},
		{"uppercase icloud", "info_at_x_com_c0w3bbrdmfw0h8_2dqb9014@iCloud.com", "info@x.com"},
		{"plain icloud", "someone@icloud.com", ""},
		{"other domain", "noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@example.com", ""},
		{"missing tokens", "info_at_x_com@icloud.com", ""},
		{"no dot in domain", "info_at_localhost_c0w3bbrdmfw0h8_2dqb9014@icloud.com", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecodeRelaySender(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSetSenderFromHeader(t *testing.T) {
	var m Message
	m.setSenderFromHeader(`"OpenAI" <noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@icloud.com>`)
	if m.FromName != "OpenAI" || m.From != "noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@icloud.com" || m.OriginalFrom != "noreply@tm.openai.com" {
		t.Fatalf("got %+v", m)
	}

	m = Message{}
	m.setSenderFromHeader("OpenAI")
	if m.From != "OpenAI" || m.FromName != "" || m.OriginalFrom != "" {
		t.Fatalf("無法解析時應整段當位址: %+v", m)
	}
}
