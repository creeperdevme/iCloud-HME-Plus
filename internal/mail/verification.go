package mail

import (
	"regexp"
	"strings"
)

var verificationCodeRE = regexp.MustCompile(`(?i)(?:verification|verify|security|confirmation|認證|驗證|驗証|驗證碼|驗証碼|code|otp|pin)[^0-9]{0,40}([0-9]{4,8})\b`)
var standaloneCodeRE = regexp.MustCompile(`\b([0-9]{6})\b`)

// ExtractVerificationCode finds a likely one-time verification code in subject/body.
func ExtractVerificationCode(subject, body string) string {
	text := strings.Join([]string{subject, body}, "\n")
	if m := verificationCodeRE.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	if m := standaloneCodeRE.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return ""
}
