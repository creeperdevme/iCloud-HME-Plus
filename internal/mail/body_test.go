package mail

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"strings"
	"testing"
)

// parseBody 是測試輔助:把原始郵件字串走完與正式路徑相同的解析。
func parseBody(t *testing.T, raw string) string {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage 失敗：%v", err)
	}
	body, err := readBody(msg)
	if err != nil {
		t.Fatalf("readBody 失敗：%v", err)
	}
	return body
}

const wantCode = "Your GitHub launch code is 12345678"

// TestReadBodyMultipartAlternativePrefersPlain 驗證 multipart/alternative 會採用
// text/plain 版本,而不是把整份 MIME 原始碼當成正文。
func TestReadBodyMultipartAlternativePrefersPlain(t *testing.T) {
	raw := `From: a@example.com
To: b@icloud.com
Subject: code
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="BOUND"

--BOUND
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

Your GitHub launch code is 12345678
--BOUND
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: quoted-printable

<html><body><p>Your GitHub launch code is <b>12345678</b></p></body></html>
--BOUND--
`
	if got := parseBody(t, raw); got != wantCode {
		t.Fatalf("正文不正確\n得到: %q\n期望: %q", got, wantCode)
	}
}

// TestReadBodyMultipartFallsBackToHTML 驗證 text/plain 部分為空時退回 HTML。
func TestReadBodyMultipartFallsBackToHTML(t *testing.T) {
	raw := `From: a@example.com
Subject: code
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="B"

--B
Content-Type: text/plain; charset=utf-8

   
--B
Content-Type: text/html; charset=utf-8

<html><body><p>Your GitHub launch code is <b>12345678</b></p></body></html>
--B--
`
	if got := parseBody(t, raw); got != wantCode {
		t.Fatalf("應退回 HTML\n得到: %q\n期望: %q", got, wantCode)
	}
}

// TestReadBodyMultipartHTMLOnly 驗證只有 HTML 的 multipart/related。
func TestReadBodyMultipartHTMLOnly(t *testing.T) {
	raw := `From: a@example.com
Subject: code
MIME-Version: 1.0
Content-Type: multipart/related; boundary="R"

--R
Content-Type: text/html; charset=utf-8

<html><head><style>.x{color:red}</style></head><body><p>Your GitHub launch code is <b>12345678</b></p></body></html>
--R--
`
	if got := parseBody(t, raw); got != wantCode {
		t.Fatalf("HTML 版本解析失敗\n得到: %q", got)
	}
}

// TestReadBodyDecodesBase64Part 驗證 base64 傳輸編碼會被解開（舊版完全沒處理）。
func TestReadBodyDecodesBase64Part(t *testing.T) {
	html := "<html><body><p>Your GitHub launch code is <b>12345678</b></p></body></html>"
	raw := fmt.Sprintf(`From: a@example.com
Subject: code
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="B64"

--B64
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: base64

%s
--B64--
`, base64.StdEncoding.EncodeToString([]byte(html)))

	if got := parseBody(t, raw); got != wantCode {
		t.Fatalf("base64 未正確解碼\n得到: %q", got)
	}
}

// TestReadBodyDecodesNonUTF8Charset 驗證非 UTF-8 字集會被轉成 UTF-8。
func TestReadBodyDecodesNonUTF8Charset(t *testing.T) {
	// iso-8859-1 的 0xE9 是 é
	raw := "From: a@example.com\nSubject: c\nContent-Type: text/plain; charset=iso-8859-1\n\ncaf\xe9 12345678\n"
	if got := parseBody(t, raw); got != "café 12345678" {
		t.Fatalf("字集轉換失敗\n得到: %q", got)
	}
}

// TestReadBodyNestedMultipart 驗證多層 multipart（alternative 內含 related）。
func TestReadBodyNestedMultipart(t *testing.T) {
	raw := `From: a@example.com
Subject: code
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="OUTER"

--OUTER
Content-Type: multipart/alternative; boundary="INNER"

--INNER
Content-Type: text/plain; charset=utf-8

Your GitHub launch code is 12345678
--INNER
Content-Type: text/html; charset=utf-8

<html><body>Your GitHub launch code is <b>12345678</b></body></html>
--INNER--
--OUTER
Content-Type: application/pdf; name="x.pdf"
Content-Transfer-Encoding: base64

JVBERi0xLjQK
--OUTER--
`
	if got := parseBody(t, raw); got != wantCode {
		t.Fatalf("巢狀 multipart 解析失敗\n得到: %q", got)
	}
}

// TestReadBodyIgnoresAttachmentOnly 驗證只有附件時不會把附件內容當成正文。
func TestReadBodyIgnoresAttachmentOnly(t *testing.T) {
	raw := `From: a@example.com
Subject: files
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="M"

--M
Content-Type: application/pdf; name="x.pdf"
Content-Transfer-Encoding: base64

JVBERi0xLjQK
--M--
`
	if got := parseBody(t, raw); got != "" {
		t.Fatalf("附件不應成為正文,得到: %q", got)
	}
}

// TestReadBodyMultipartWithInlineStylesIsNotEmpty 是這次問題的迴歸測試。
//
// 真實信件（GitHub 通知、行銷信）的 HTML 大量使用 inline style 與 mso- 屬性。
// 舊版把整份 multipart 原始碼當純文字,再被 sanitizePlainPreview 誤判為 CSS 而
// 清成空字串,前端顯示「(無內文)」。這裡要求一定要拿到真正的文字。
func TestReadBodyMultipartWithInlineStylesIsNotEmpty(t *testing.T) {
	raw := `From: noreply_at_github_com@icloud.com
To: rondo_fudge_4x@icloud.com
Subject: =?UTF-8?Q?=F0=9F=9A=80_Your_GitHub_launch_code?=
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="Apple-Mail-XYZ"

--Apple-Mail-XYZ
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

Your GitHub launch code is 12345678
--Apple-Mail-XYZ
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: quoted-printable

<html><head><style type=3D"text/css">.a{margin:0;padding:0}</style></head>
<body style=3D"margin:0;padding:0;font-family:Helvetica,Arial,sans-serif;background-color:#f6f8fa">
<table style=3D"border-collapse:collapse;width:100%;mso-table-lspace:0pt">
<tr><td style=3D"font-family:Helvetica;text-size-adjust:none;-webkit-text-size-adjust:none">GitHub launch code is <b>12345678</b></td></tr>
</table></body></html>
--Apple-Mail-XYZ--
`
	got := parseBody(t, raw)
	if got == "" {
		t.Fatal("正文不得為空（舊版會回空字串,導致前端顯示「(無內文)」）")
	}
	if !strings.Contains(got, "12345678") {
		t.Fatalf("正文應包含驗證碼,得到: %q", got)
	}
	if strings.Contains(got, "--Apple-Mail-XYZ") || strings.Contains(got, "Content-Type:") {
		t.Fatalf("正文不應殘留 MIME 原始碼,得到: %q", got)
	}
}

// TestReadBodySimpleMessages 確認原本就支援的單一 part 訊息沒有退化。
func TestReadBodySimpleMessages(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"text/plain", "From: a@example.com\nSubject: s\nContent-Type: text/plain; charset=utf-8\n\n" + wantCode + "\n"},
		{"text/html", "From: a@example.com\nSubject: s\nContent-Type: text/html; charset=utf-8\n\n<html><body><p>" + wantCode + "</p></body></html>\n"},
		{"quoted-printable", "From: a@example.com\nSubject: s\nContent-Type: text/plain; charset=utf-8\nContent-Transfer-Encoding: quoted-printable\n\nYour GitHub launch code is 1234=\n5678\n"},
		{"無 Content-Type", "From: a@example.com\nSubject: s\n\n" + wantCode + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBody(t, tc.raw); got != wantCode {
				t.Fatalf("得到: %q\n期望: %q", got, wantCode)
			}
		})
	}
}
