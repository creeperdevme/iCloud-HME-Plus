// Package server - 可替換業務介面與 Manager 適配器。
//
// Backend 邊界固定為高層業務動作,不把具體 *hme.Client 或 *mail.Client 暴露給 handler。
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/mail"
)

// BackendError 是後端返回的穩定錯誤,攜帶 HTTP 狀態碼與穩定錯誤碼。
type BackendError struct {
	Status  int
	Code    string
	Message string
}

func (e *BackendError) Error() string { return e.Message }

// InboxQuery 是收件匣查詢參數。
type InboxQuery struct {
	AccountID string
	Alias     string
	Limit     int
	Days      int
}

// InboxResult 是收件匣查詢結果。
type InboxResult struct {
	AccountID string         `json:"account_id"`
	Alias     string         `json:"alias,omitempty"`
	Count     int            `json:"count"`
	Messages  []mail.Message `json:"messages"`
	Method    string         `json:"method"`
}

// Backend 是可替換的業務介面;handler 只依賴本介面,測試使用記憶體 fake。
type Backend interface {
	ListAccounts() []account.Summary
	// GetAccount 取單一帳號的安全摘要;第二個回傳值表示是否存在。
	GetAccount(string) (account.Summary, bool)
	AddAccount(account.AddAccountInput) (account.Summary, error)
	UpdateAccount(string, account.UpdateAccountInput) (account.Summary, error)
	UpdateProxy(string, string) (account.Summary, error)
	UpdateCookies(string, string) (account.Summary, error)
	SetAppPassword(string, string, string) (account.Summary, error)
	SetMailbox(string, account.MailboxConfig) (account.Summary, error)
	LoginAccount(string, string, string) (account.Summary, error)
	RemoveAccount(string) bool
	CreateAlias(string, string) (*hme.CreateResult, error)
	ListAliases(string) ([]hme.Alias, error)
	SetAliasActive(string, string, bool) (bool, error)
	DeleteAlias(string, string) error
	ListInbox(InboxQuery) (InboxResult, error)
	GetMessage(string, uint32) (*mail.FullMessage, error)
	DeleteMessage(string, uint32) error
	Reload() error
}

// managerBackend 是生產 Backend,包裝 *account.Manager。
type managerBackend struct {
	mgr *account.Manager
}

// ListAccounts 返回帳號安全摘要列表。
func (b *managerBackend) ListAccounts() []account.Summary {
	return b.mgr.ListSummaries()
}

// GetAccount 返回單一帳號的安全摘要。
func (b *managerBackend) GetAccount(id string) (account.Summary, bool) {
	acc, ok := b.mgr.GetAccount(id)
	if !ok {
		return account.Summary{}, false
	}
	return acc.Summary(), true
}

// AddAccount 新增帳號。
func (b *managerBackend) AddAccount(in account.AddAccountInput) (account.Summary, error) {
	sum, err := b.mgr.AddAccountWithInput(in)
	if err != nil {
		return account.Summary{}, &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: err.Error()}
	}
	return sum, nil
}

// UpdateAccount 編輯帳號基本資訊。
func (b *managerBackend) UpdateAccount(id string, in account.UpdateAccountInput) (account.Summary, error) {
	sum, err := b.mgr.UpdateMetadata(id, in)
	if err != nil {
		return account.Summary{}, mapAccountErr(err)
	}
	return sum, nil
}

// UpdateProxy 更新或清除帳號代理。
func (b *managerBackend) UpdateProxy(id, proxy string) (account.Summary, error) {
	sum, err := b.mgr.UpdateProxy(id, proxy)
	if err != nil {
		return account.Summary{}, mapAccountErr(err)
	}
	return sum, nil
}

// UpdateCookies 更新帳號 Cookie。cookies 為原始文本(Header String 或 JSON)。
func (b *managerBackend) UpdateCookies(id, cookies string) (account.Summary, error) {
	parsed, err := account.ParseCookieInput(cookies)
	if err != nil {
		return account.Summary{}, &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: err.Error()}
	}
	if err := b.mgr.UpdateCookies(id, parsed); err != nil {
		return account.Summary{}, mapAccountErr(err)
	}
	sum, ok := b.mgr.GetAccount(id)
	if !ok {
		return account.Summary{}, &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
	}
	return sum.Summary(), nil
}

// SetAppPassword 設定 iCloud 信箱與 App 專用密碼並測試 IMAP 連線。
func (b *managerBackend) SetAppPassword(id, icloudEmail, appPassword string) (account.Summary, error) {
	if err := b.mgr.SetAppPassword(id, icloudEmail, appPassword); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "帳號不存在") {
			return account.Summary{}, &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
		}
		if strings.Contains(msg, "不能為空") {
			return account.Summary{}, &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: msg}
		}
		// IMAP 連線失敗屬於上游錯誤,不拼接詳細錯誤
		return account.Summary{}, &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "IMAP 驗證失敗，請檢查信箱與 App 專用密碼"}
	}
	sum, ok := b.mgr.GetAccount(id)
	if !ok {
		return account.Summary{}, &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
	}
	return sum.Summary(), nil
}

// SetMailbox configures and verifies an external IMAP mailbox.
func (b *managerBackend) SetMailbox(id string, config account.MailboxConfig) (account.Summary, error) {
	if err := b.mgr.SetMailbox(id, config); err != nil {
		if strings.Contains(err.Error(), "帳號不存在") {
			return account.Summary{}, &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
		}
		return account.Summary{}, &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "收件信箱驗證失敗，請檢查信箱、授權碼與 IMAP 設定"}
	}
	sum, ok := b.mgr.GetAccount(id)
	if !ok {
		return account.Summary{}, &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
	}
	return sum.Summary(), nil
}

// LoginAccount 使用 iCloud 密碼登入帳號,成功只返回 Summary,絕不返回 Cookies。
func (b *managerBackend) LoginAccount(id, password, otpCode string) (account.Summary, error) {
	var otpProvider hme.OTPProvider
	if otpCode != "" {
		otp := otpCode
		otpProvider = func() (string, error) { return otp, nil }
	}

	client, err := b.mgr.HMEClientWithPassword(id, password, otpProvider)
	if err != nil {
		return account.Summary{}, classifyLoginErr(err)
	}
	_ = client
	sum, ok := b.mgr.GetAccount(id)
	if !ok {
		return account.Summary{}, &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
	}
	return sum.Summary(), nil
}

// classifyLoginErr 把 iCloud 登入錯誤映射為穩定錯誤。
//
// 注意:這裡絕不能返回 HTTP 401。401 是本服務"管理員工作階段失效"的專用信號,
// 前端 fetch 封裝收到 401 會清空工作階段並跳回登入頁。iCloud 帳號登入時輸錯
// OTP/密碼屬於業務錯誤,必須用其它狀態碼表達。
func classifyLoginErr(err error) *BackendError {
	msg := err.Error()
	if strings.Contains(msg, "需要提供 OTP") {
		return &BackendError{Status: http.StatusConflict, Code: "OTP_REQUIRED", Message: "需要提供 OTP 驗證碼"}
	}
	// 產生端（internal/hme/auth.go）已改為繁體「2FA 驗證失敗」；
	// 同時保留簡體比對，避免舊版或上游文字漏判。
	if strings.Contains(msg, "2FA 驗證失敗") || strings.Contains(msg, "2FA 验证失败") {
		return &BackendError{Status: http.StatusBadRequest, Code: "OTP_INVALID", Message: "OTP 驗證碼錯誤"}
	}
	if strings.Contains(msg, "帳號不存在") {
		return &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
	}
	if isSessionError(msg) {
		return upstreamUnauthorizedErr()
	}
	return &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "iCloud 登入失敗，請稍後再試"}
}

// RemoveAccount 刪除帳號。
func (b *managerBackend) RemoveAccount(id string) bool {
	return b.mgr.RemoveAccount(id)
}

// CreateAlias 建立 HME 別名。
func (b *managerBackend) CreateAlias(accountID, label string) (*hme.CreateResult, error) {
	client, err := b.mgr.HMEClient(accountID, false)
	if err != nil {
		return nil, mapAccountErr(err)
	}
	result, err := client.CreateAlias(label, 5)
	_ = b.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		return nil, classifyUpstreamErr("建立信箱失敗", err)
	}
	return result, nil
}

// ListAliases 列出帳號的 HME 別名。
func (b *managerBackend) ListAliases(accountID string) ([]hme.Alias, error) {
	client, err := b.mgr.HMEClient(accountID, false)
	if err != nil {
		return nil, mapAccountErr(err)
	}
	aliases, err := client.ListAliases()
	_ = b.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		return nil, classifyUpstreamErr("取得別名列表失敗", err)
	}
	return aliases, nil
}

// SetAliasActive 停用或激活別名。
func (b *managerBackend) SetAliasActive(accountID, anonymousID string, active bool) (bool, error) {
	client, err := b.mgr.HMEClient(accountID, false)
	if err != nil {
		return false, mapAccountErr(err)
	}
	var success bool
	if active {
		success, err = client.ReactivateHME(anonymousID)
	} else {
		success, err = client.DeactivateHME(anonymousID)
	}
	_ = b.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		msg := "操作失敗"
		if !active {
			msg = "停用失敗"
		} else {
			msg = "啟用失敗"
		}
		return false, classifyUpstreamErr(msg, err)
	}
	return success, nil
}

// DeleteAlias 刪除別名。
func (b *managerBackend) DeleteAlias(accountID, anonymousID string) error {
	client, err := b.mgr.HMEClient(accountID, false)
	if err != nil {
		return mapAccountErr(err)
	}
	err = client.Delete(anonymousID)
	_ = b.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		return classifyUpstreamErr("刪除失敗", err)
	}
	return nil
}

// ListInbox 讀取收件匣摘要:IMAP (App Password) 優先,Web API (Cookie) 回退。
func (b *managerBackend) ListInbox(q InboxQuery) (InboxResult, error) {
	// 優先使用 IMAP 連線池 (App Password 認證,複用長連線)
	var imapMessages []mail.Message
	poolErr := b.mgr.WithMailClient(q.AccountID, func(mc *mail.Client) error {
		var e error
		if q.Alias != "" {
			imapMessages, e = mc.FindByRecipient(q.Alias, q.Limit, q.Days)
		} else {
			imapMessages, e = mc.ListInbox(q.Limit, q.Days)
		}
		return e
	})
	if poolErr == nil {
		return InboxResult{
			AccountID: q.AccountID,
			Alias:     q.Alias,
			Count:     len(imapMessages),
			Messages:  imapMessages,
			Method:    "imap",
		}, nil
	}
	// IMAP 失敗,繼續嘗試 Web API

	// 回退到 Web API (Cookie 認證,無需 App Password)
	wmc, err := b.mgr.WebMailClient(q.AccountID)
	if err != nil {
		return InboxResult{}, &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: "沒有可用的郵件用戶端：需要 App 專用密碼或 Cookie"}
	}

	if q.Alias != "" {
		messages, err := wmc.FindByAlias(q.Alias, q.Limit)
		if err != nil {
			return InboxResult{}, &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "讀取郵件失敗"}
		}
		return InboxResult{AccountID: q.AccountID, Alias: q.Alias, Count: len(messages), Messages: messages, Method: "web_api"}, nil
	}
	messages, err := wmc.ListInbox(q.Limit)
	if err != nil {
		return InboxResult{}, &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "讀取郵件失敗"}
	}
	return InboxResult{AccountID: q.AccountID, Count: len(messages), Messages: messages, Method: "web_api"}, nil
}

func (b *managerBackend) GetMessage(accountID string, uid uint32) (*mail.FullMessage, error) {
	mc, err := b.mgr.MailClient(accountID)
	if err != nil {
		return nil, mapAccountErr(err)
	}
	if err := mc.Connect(); err != nil {
		return nil, &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "讀取郵件失敗"}
	}
	defer mc.Disconnect()
	message, err := mc.GetFull(uid)
	if err != nil {
		return nil, &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "讀取郵件內容失敗"}
	}
	return message, nil
}

func (b *managerBackend) DeleteMessage(accountID string, uid uint32) error {
	mc, err := b.mgr.MailClient(accountID)
	if err != nil {
		return mapAccountErr(err)
	}
	if err := mc.Connect(); err != nil {
		return &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "刪除郵件失敗"}
	}
	defer mc.Disconnect()
	if err := mc.Delete(uid); err != nil {
		return &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: "刪除郵件失敗"}
	}
	return nil
}

// Reload 重新載入配置。
func (b *managerBackend) Reload() error {
	if err := b.mgr.Reload(); err != nil {
		return &BackendError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "重新載入設定失敗"}
	}
	return nil
}

// mapAccountErr 把帳號管理器錯誤映射為穩定錯誤。
func mapAccountErr(err error) *BackendError {
	msg := err.Error()
	if strings.Contains(msg, "帳號不存在") {
		return &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "帳號不存在"}
	}
	// 產生端（internal/account/manager.go）已改為繁體「尚未設定 Cookie」，
	// 因此三種寫法都要比對，否則會退化成把內部錯誤原文直接回給前端。
	if strings.Contains(msg, "Cookie") &&
		(strings.Contains(msg, "尚未設定") || strings.Contains(msg, "未配置") || strings.Contains(msg, "未設置")) {
		return &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: "帳號尚未設定 Cookie"}
	}
	return &BackendError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: msg}
}

// upstreamUnauthorizedErr 表示"上游 iCloud 工作階段失效"。
//
// 關鍵約定:上游鑑權失敗必須返回 502,而不是 401。
// 本服務的 401 只表示管理員工作階段失效(見 internal/server/auth.go 的
// requireSession/handleSession),前端 api/client.ts 一旦收到 401 就會
// 清空 CSRF token 並把介面切回登入頁。若上游錯誤也用 401,那麼任何
// iCloud Cookie 過期(約 24 小時)、或輸錯一次 OTP,都會把管理員踢出面板。
func upstreamUnauthorizedErr() *BackendError {
	return &BackendError{
		Status:  http.StatusBadGateway,
		Code:    "UPSTREAM_UNAUTHORIZED",
		Message: "iCloud 工作階段已失效，請更新 Cookie",
	}
}

// classifyUpstreamErr 把上游 (iCloud) 錯誤映射為穩定錯誤,不拼接上游回應體。
func classifyUpstreamErr(fixedMsg string, err error) *BackendError {
	if err == nil {
		return nil
	}
	if isSessionError(err.Error()) {
		return upstreamUnauthorizedErr()
	}
	return &BackendError{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILURE", Message: fixedMsg}
}

// isSessionError 判斷錯誤是否由工作階段失效引起。
//
// 「認證」與「工作階段校驗失敗」是上游 iCloud 直接回傳的簡體字串，我們無法改寫，
// 因此同時保留簡體與繁體兩種比對；其餘字串皆由本專案產生（已全面轉為繁體），
// 比對條件也一併使用繁體。
func isSessionError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "401") || strings.Contains(m, "403") ||
		strings.Contains(m, "session") || strings.Contains(m, "cookie") ||
		strings.Contains(m, "unauthorized") ||
		strings.Contains(m, "认证") || strings.Contains(m, "認證") ||
		strings.Contains(m, "会话校验失败") || strings.Contains(m, "工作階段校驗失敗")
}

// asBackendError 提取 BackendError,非 BackendError 統一為 INTERNAL_ERROR。
func asBackendError(err error) *BackendError {
	var be *BackendError
	if errors.As(err, &be) {
		return be
	}
	return &BackendError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "內部錯誤"}
}

// cookieInputToJSON 把 handler 解析出的 map 轉回 JSON 文本,交給 ParseCookieInput。
func cookieInputToJSON(cookies map[string]string) string {
	raw, err := json.Marshal(cookies)
	if err != nil {
		return ""
	}
	return string(raw)
}
