package mail

import (
	"net/mail"
	"regexp"
	"strings"
)

// relaySenderRE 比對 iCloud 隱藏郵件地址轉發時改寫的寄件人:
//
//	noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@icloud.com → noreply@tm.openai.com
//
// 格式為「原帳號_at_網域(點換成底線)_亂碼_亂碼」。Apple 沒有公開這個格式,
// 以上是從實際信件歸納的;比對不到就不還原,呼叫端照常顯示改寫後的位址。
//
// 原帳號本身含 _at_ 時無法與網域中的 at 區分,這裡取第一個 _at_。
var relaySenderRE = regexp.MustCompile(`^(.+?)_at_([A-Za-z0-9-]+(?:_[A-Za-z0-9-]+)+)_[a-z0-9]+_[a-z0-9]+$`)

// DecodeRelaySender 把 iCloud 轉發改寫的寄件人還原成原始位址;不是轉發位址時回傳空字串。
func DecodeRelaySender(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at < 0 || !strings.EqualFold(addr[at+1:], "icloud.com") {
		return ""
	}
	m := relaySenderRE.FindStringSubmatch(addr[:at])
	if m == nil {
		return ""
	}
	// 網域不允許底線,所以網域裡的底線一定是原本的點。
	return m[1] + "@" + strings.ReplaceAll(m[2], "_", ".")
}

// setSender 填入寄件人位址、顯示名稱與還原後的原始位址。
func (m *Message) setSender(name, addr string) {
	m.From = addr
	m.FromName = strings.TrimSpace(name)
	m.OriginalFrom = DecodeRelaySender(addr)
}

// setSenderFromHeader 解析 "Name <addr>" 形式的寄件人;解析不了就整段當作位址。
func (m *Message) setSenderFromHeader(s string) {
	if a, err := mail.ParseAddress(s); err == nil {
		m.setSender(a.Name, a.Address)
		return
	}
	m.setSender("", s)
}
