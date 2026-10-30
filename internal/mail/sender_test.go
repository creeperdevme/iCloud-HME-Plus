package mail

import "testing"

func TestDecodeRelaySender(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"subdomain", "noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@icloud.com", "noreply@tm.openai.com"},
		{"mixed case local", "no-reply-mdPUDTvILQoXIafocg23Zg_at_mail_anthropic_com_4y4w6cpt990858_b416bc66@icloud.com", "no-reply-mdPUDTvILQoXIafocg23Zg@mail.anthropic.com"},
		{"short domain", "info_at_x_com_c0w3bbrdmfw0h8_2dqb9014@icloud.com", "info@x.com"},
		{"underscore in local", "no_reply_at_github_com_c0w3bbrdmfw0h8_2dqb9014@icloud.com", "no_reply@github.com"},
		// 以下三筆是實際寄信取得的轉發位址:Apple 原樣保留帳號裡的 . + _。
		{"dot in local", "john.doe_at_inboxmail_cc_cd_hr247g9fbd88sr_472q0858@icloud.com", "john.doe@inboxmail.cc.cd"},
		{"underscore local real", "john_doe_at_inboxmail_cc_cd_hr787ge9bd25sr_8a2q0858@icloud.com", "john_doe@inboxmail.cc.cd"},
		{"plus in local", "johndoe+tag1_at_inboxmail_cc_cd_hr447g02bd7asr_ef2q0858@icloud.com", "johndoe+tag1@inboxmail.cc.cd"},
		{"_at_ in local", "john_at_doe_at_inboxmail_cc_cd_hr247g9fbd88sr_472q0858@icloud.com", "john_at_doe@inboxmail.cc.cd"},
		{"relay of relay", "jane_at_gmail_com_x7k2_at_simplelogin_co_hr247g9fbd88sr_472q0858@icloud.com", "jane_at_gmail_com_x7k2@simplelogin.co"},
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
