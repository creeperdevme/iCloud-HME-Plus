// Package account - 公開帳號 DTO 與輸入校驗。
//
// HTTP 層只能序列化 account.Summary;內部 Account(含 Cookies、AppPassword、
// Proxy 等秘密)只用於持久化和內部用戶端構造,絕不直接出現在回應中。
package account

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
)

// Summary 是帳號的安全公開表示,不含任何秘密欄位。
type Summary struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	RealEmail      string          `json:"real_email"`
	ICloudEmail    string          `json:"icloud_email"`
	Host           string          `json:"host"`
	Status         string          `json:"status"`
	AliasTotal     int             `json:"alias_total"`
	AliasActive    int             `json:"alias_active"`
	HasCookies     bool            `json:"has_cookies"`
	HasAppPassword bool            `json:"has_app_password"`
	HasProxy       bool            `json:"has_proxy"`
	Mailbox        *MailboxSummary `json:"mailbox,omitempty"`
	LastValidated  string          `json:"last_validated"`
	StatusMessage  string          `json:"status_message,omitempty"`
	CreatedAt      string          `json:"created_at"`
}

// Summary 返回帳號的安全快照,忽略內部 LastError。
func (a *Account) Summary() Summary {
	s := Summary{
		ID:             a.ID,
		Name:           a.Name,
		RealEmail:      a.RealEmail,
		ICloudEmail:    a.ICloudEmail,
		Host:           a.Host,
		Status:         a.Status,
		AliasTotal:     a.AliasTotal,
		AliasActive:    a.AliasActive,
		HasCookies:     len(a.Cookies) > 0,
		HasAppPassword: a.AppPassword != "",
		HasProxy:       a.Proxy != "",
		LastValidated:  a.LastValidated,
		CreatedAt:      a.CreatedAt,
	}
	if a.Mailbox != nil {
		s.Mailbox = &MailboxSummary{Provider: a.Mailbox.Provider, Email: a.Mailbox.Email, IMAPHost: a.Mailbox.IMAPHost, IMAPPort: a.Mailbox.IMAPPort}
	}
	switch a.Status {
	case "pending":
		s.StatusMessage = "等待設定或驗證憑證"
	case "error":
		s.StatusMessage = "憑證驗證失敗"
	}
	return s
}

type MailboxSummary struct {
	Provider string `json:"provider"`
	Email    string `json:"email"`
	IMAPHost string `json:"imap_host"`
	IMAPPort int    `json:"imap_port"`
}

// AddAccountInput 是新增帳號的輸入。
type AddAccountInput struct {
	Name        string
	ICloudEmail string
	CookieInput string
	Host        string
	Proxy       string
}

// UpdateAccountInput 是編輯帳號基本資訊的輸入,指標欄位表示可選。
type UpdateAccountInput struct {
	Name        *string
	ICloudEmail *string
	Host        *string
}

// validateName 校驗名稱:去空白後 1–64 字元。
func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("名稱為必填")
	}
	if len([]rune(name)) > 64 {
		return "", fmt.Errorf("名稱不能超過 64 個字元")
	}
	return name, nil
}

// validateHost 校驗主機:只能是 icloud.com 或 icloud.com.cn。
func validateHost(host string) (string, error) {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return "icloud.com", nil
	}
	if host != "icloud.com" && host != "icloud.com.cn" {
		return "", fmt.Errorf("主機只能是 icloud.com 或 icloud.com.cn")
	}
	return host, nil
}

// validateEmail 校驗信箱:用 net/mail.ParseAddress 並要求位址值等於輸入。
func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("iCloud 信箱不能為空")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("信箱格式無效")
	}
	if addr.Address != email {
		return fmt.Errorf("信箱格式無效")
	}
	return nil
}

// validEmailLocalPart 判斷字串是否可作為信箱本地部分(Prefix)。
//
// 只做保守校驗:非空、不含空白與 @、不含路徑分隔符等明顯非法字元。
func validEmailLocalPart(local string) bool {
	if local == "" || len([]rune(local)) > 64 {
		return false
	}
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '+':
		default:
			return false
		}
	}
	return true
}

// NormalizeICloudEmail 把使用者輸入規範化為完整 iCloud 信箱。
//
// 支援兩種輸入:
//   - 完整信箱: "owner@icloud.com"   → 原樣返回(校驗格式)
//   - 僅 Prefix: "owner"             → 補全為 owner@<host>
//
// host 必須是 validateHost 已校驗過的值。
func NormalizeICloudEmail(raw, host string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("iCloud 信箱不能為空")
	}
	if strings.Contains(raw, "@") {
		if err := validateEmail(raw); err != nil {
			return "", err
		}
		return raw, nil
	}
	if !validEmailLocalPart(raw) {
		return "", fmt.Errorf("iCloud 信箱 Prefix 格式無效")
	}
	if host == "" {
		host = "icloud.com"
	}
	return raw + "@" + host, nil
}

// DefaultAccountName 從 iCloud 信箱推導帳號顯示名稱(@ 前的本地部分)。
//
// 用於"新增帳號不必填名稱"的場景;推導結果保證非空且不超過 64 字元。
func DefaultAccountName(email string) string {
	local := strings.TrimSpace(email)
	if idx := strings.Index(local, "@"); idx > 0 {
		local = local[:idx]
	}
	if local == "" {
		local = "iCloud 帳號"
	}
	runes := []rune(local)
	if len(runes) > 64 {
		local = string(runes[:64])
	}
	return local
}

// validateProxy 校驗代理;空表示清除。
func validateProxy(proxy string) (string, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return "", nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("代理位址格式無效")
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return "", fmt.Errorf("代理位址格式無效")
	}
	return proxy, nil
}
