package mail

import "testing"

func TestExtractVerificationCode(t *testing.T) {
	cases := []struct{ name, subject, body, want string }{
		{"label", "Your verification code", "Use code 482913 to continue", "482913"},
		{"chinese", "登入驗證碼", "驗證碼：731204", "731204"},
		{"fallback", "Security alert", "One-time number: 123456", "123456"},
		{"none", "Newsletter", "No code here", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractVerificationCode(tc.subject, tc.body); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
