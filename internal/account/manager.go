// Package account 實現多帳號管理器。
//
// 負責帳號 CRUD、Cookie 解析(Header String / JSON)、持久化到 accounts.json,
// 以及建立 HME 用戶端和郵件用戶端。對應原 Python 項目 account_manager.py。
package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/mail"
)

// Account 描述一個 iCloud 帳號。
type Account struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	RealEmail     string            `json:"real_email"`
	ICloudEmail   string            `json:"icloud_email"`
	Cookies       map[string]string `json:"cookies"`
	Host          string            `json:"host"`
	Proxy         string            `json:"proxy,omitempty"` // HTTP/SOCKS5 代理
	AppPassword   string            `json:"app_password,omitempty"`
	Mailbox       *MailboxConfig    `json:"mailbox,omitempty"`
	Status        string            `json:"status"` // active / error
	AliasTotal    int               `json:"alias_total"`
	AliasActive   int               `json:"alias_active"`
	LastValidated string            `json:"last_validated"`
	LastError     string            `json:"last_error,omitempty"`
	CreatedAt     string            `json:"created_at"`
}

// MailboxConfig describes an external mailbox used to receive forwarded mail.
type MailboxConfig struct {
	Provider string `json:"provider"`
	Email    string `json:"email"`
	IMAPHost string `json:"imap_host"`
	IMAPPort int    `json:"imap_port"`
	Password string `json:"password,omitempty"`
}

// Manager 管理多個 iCloud 帳號,執行緒安全。
type Manager struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	dataDir  string
	dataFile string
	imapPool *mail.Pool // IMAP 長連線池
}

// cloneCookies 返回 Cookie map 的獨立副本。
func cloneCookies(cookies map[string]string) map[string]string {
	if cookies == nil {
		return nil
	}
	cloned := make(map[string]string, len(cookies))
	for k, v := range cookies {
		cloned[k] = v
	}
	return cloned
}

// copyAccount 返回帳號的深拷貝(含 Cookies map),必須在持鎖時呼叫。
func copyAccount(acc *Account) *Account {
	if acc == nil {
		return nil
	}
	cp := *acc
	cp.Cookies = cloneCookies(acc.Cookies)
	return &cp
}

// NewManager 建立管理器。dataDir 用於存放 accounts.json。
func NewManager(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	m := &Manager{
		accounts: make(map[string]*Account),
		dataDir:  dataDir,
		dataFile: filepath.Join(dataDir, "accounts.json"),
		imapPool: mail.NewPool(),
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

// Close 釋放 IMAP 連線池等資源。
func (m *Manager) Close() {
	if m.imapPool != nil {
		m.imapPool.Close()
	}
}

// Reload 重新載入 accounts.json 配置檔案。
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.load()
}

func (m *Manager) load() error {
	raw, err := os.ReadFile(m.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var wrapper struct {
		Accounts map[string]*Account `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return err
	}
	m.accounts = wrapper.Accounts
	if m.accounts == nil {
		m.accounts = make(map[string]*Account)
	}
	return nil
}

func (m *Manager) save() error {
	wrapper := struct {
		Accounts  map[string]*Account `json:"accounts"`
		UpdatedAt string              `json:"updated_at"`
	}{
		Accounts:  m.accounts,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.dataFile, raw, 0600)
}

// ParseCookieInput 解析 Cookie 輸入,支援兩種格式:
//   - Header String: "name1=value1; name2=value2; ..."
//   - JSON: {"name1":"value1","name2":"value2"}
//
// 空輸入返回錯誤。
func ParseCookieInput(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("輸入為空 — 請貼上 Cookie Header String 或 JSON")
	}

	// JSON 格式
	if strings.HasPrefix(raw, "{") {
		var cookies map[string]string
		if err := json.Unmarshal([]byte(raw), &cookies); err == nil && cookies != nil {
			out := make(map[string]string, len(cookies))
			for k, v := range cookies {
				if v != "" {
					out[k] = v
				}
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}

	// Header String 格式
	cookies := make(map[string]string)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, "=")
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if name != "" {
			cookies[name] = value
		}
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("無法解析 Cookie 輸入，請提供 Header String 或 JSON 格式")
	}
	return cookies, nil
}

// AddAccount 新增一個帳號。cookieInput 可為空,後續可通過 /login 取得。
//
// cookieInput 支援 Header String 或 JSON。校驗失敗仍會儲存帳號(status=error),
// 方便使用者後續修正 Cookie 後重新校驗。
//
// 兼容入口:新呼叫方請使用 AddAccountWithInput。
func (m *Manager) AddAccount(name, cookieInput, host, proxy string) (*Account, error) {
	if host == "" {
		host = "icloud.com"
	}
	acc, err := m.newAccount(name, "", cookieInput, host, proxy)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.accounts[acc.ID] = acc
	saveErr := m.save()
	m.mu.Unlock()
	if saveErr != nil {
		return nil, saveErr
	}
	return acc, nil
}

// AddAccountWithInput 新增帳號(帶完整校驗)。
//
// Name 選填:留空時自動取 iCloud 信箱 Prefix 作為顯示名稱。
// ICloudEmail 兼容兩種輸入:完整信箱,或僅 Prefix(自動補全 @host)。
//
// 無 Cookie 的新增路徑不訪問網路;有 Cookie 時在鎖外對快照執行工作階段校驗。
func (m *Manager) AddAccountWithInput(input AddAccountInput) (Summary, error) {
	host, err := validateHost(input.Host)
	if err != nil {
		return Summary{}, err
	}
	email, err := NormalizeICloudEmail(input.ICloudEmail, host)
	if err != nil {
		return Summary{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = DefaultAccountName(email)
	} else if name, err = validateName(name); err != nil {
		return Summary{}, err
	}
	proxy, err := validateProxy(input.Proxy)
	if err != nil {
		return Summary{}, err
	}
	acc, err := m.newAccount(name, email, input.CookieInput, host, proxy)
	if err != nil {
		return Summary{}, err
	}
	m.mu.Lock()
	m.accounts[acc.ID] = acc
	saveErr := m.save()
	m.mu.Unlock()
	if saveErr != nil {
		return Summary{}, saveErr
	}
	return acc.Summary(), nil
}

// newAccount 構造帳號;cookieInput 非空時在鎖外對快照執行工作階段校驗。
func (m *Manager) newAccount(name, icloudEmail, cookieInput, host, proxy string) (*Account, error) {
	var cookies map[string]string
	if cookieInput != "" {
		var err error
		cookies, err = ParseCookieInput(cookieInput)
		if err != nil {
			return nil, err
		}
	} else {
		cookies = make(map[string]string)
	}

	acc := &Account{
		ID:          "acc_" + uuid.New().String()[:8],
		Name:        name,
		RealEmail:   icloudEmail,
		ICloudEmail: icloudEmail,
		Cookies:     cookies,
		Host:        host,
		Proxy:       proxy,
		Status:      "pending", // 無 Cookie 時為 pending
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	// 有 Cookie 才校驗工作階段
	if len(cookies) > 0 {
		acc.validateCookies()
	}
	return acc, nil
}

// validateCookies 用 Cookie 校驗工作階段並填充帳號身份(在鎖外對快照操作)。
func (a *Account) validateCookies() {
	host := a.Host
	if host == "" {
		host = "icloud.com"
	}
	client, err := hme.NewClient(a.Cookies, host, a.Proxy, false)
	if err != nil {
		a.Status = "error"
		a.LastError = truncate(err.Error(), 300)
		return
	}
	if err := client.ValidateSession(); err != nil {
		// validate 即使失敗也可能通過 Set-Cookie 刷新部分工作階段狀態。
		a.Cookies = client.Cookies
		a.Status = "error"
		a.LastError = truncate(err.Error(), 300)
		return
	}
	// 顯式接收 validate 刷新的 Cookie，不依賴傳入 map 的引用關係。
	a.Cookies = client.Cookies
	a.Status = "active"
	if info := client.AccountInfo(); info != nil {
		a.RealEmail = firstNonEmpty(info.AppleID, info.PrimaryEmail)
		if a.ICloudEmail == "" {
			a.ICloudEmail = deriveICloudEmail(info)
		}
	}
	if aliases, err := client.ListAliases(); err == nil {
		a.AliasTotal = len(aliases)
		for _, al := range aliases {
			if al.Active {
				a.AliasActive++
			}
		}
	}
	a.LastValidated = time.Now().Format(time.RFC3339)
}

// UpdateMetadata 編輯帳號基本資訊(名稱、iCloud 信箱、主機),至少提供一個欄位。
func (m *Manager) UpdateMetadata(id string, input UpdateAccountInput) (Summary, error) {
	if input.Name == nil && input.ICloudEmail == nil && input.Host == nil {
		return Summary{}, fmt.Errorf("至少需要提供一個可編輯欄位")
	}
	var name, email, host *string
	if input.Name != nil {
		v, err := validateName(*input.Name)
		if err != nil {
			return Summary{}, err
		}
		name = &v
	}
	if input.ICloudEmail != nil {
		// Prefix 需要帳號當前的 host 才能補全,故規範化延後到持鎖之後。
		v := strings.TrimSpace(*input.ICloudEmail)
		email = &v
	}
	if input.Host != nil {
		v, err := validateHost(*input.Host)
		if err != nil {
			return Summary{}, err
		}
		host = &v
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return Summary{}, fmt.Errorf("帳號不存在: %s", id)
	}
	if name != nil {
		acc.Name = *name
	}
	if email != nil {
		resolvedHost := acc.Host
		if host != nil {
			resolvedHost = *host
		}
		v, err := NormalizeICloudEmail(*email, resolvedHost)
		if err != nil {
			return Summary{}, err
		}
		acc.ICloudEmail = v
	}
	if host != nil {
		acc.Host = *host
	}
	if err := m.save(); err != nil {
		return Summary{}, err
	}
	return acc.Summary(), nil
}

// UpdateProxy 更新或清除帳號代理。空字串表示清除。
func (m *Manager) UpdateProxy(id, proxy string) (Summary, error) {
	proxy, err := validateProxy(proxy)
	if err != nil {
		return Summary{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return Summary{}, fmt.Errorf("帳號不存在: %s", id)
	}
	acc.Proxy = proxy
	if err := m.save(); err != nil {
		return Summary{}, err
	}
	return acc.Summary(), nil
}

// RemoveAccount 刪除帳號。
func (m *Manager) RemoveAccount(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[id]; !ok {
		return false
	}
	delete(m.accounts, id)
	_ = m.save()
	return true
}

// GetAccount 返回帳號深拷貝(含 Cookies),呼叫方可安全使用。
func (m *Manager) GetAccount(id string) (*Account, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.accounts[id]
	if !ok {
		return nil, false
	}
	return copyAccount(acc), true
}

// ListAccounts 返回所有帳號的深拷貝(脫敏,不含 Cookies),按活躍狀態排序。
// 兼容入口:新呼叫方請使用 ListSummaries。
func (m *Manager) ListAccounts() []*Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Account, 0, len(m.accounts))
	for _, acc := range m.accounts {
		cp := copyAccount(acc)
		cp.Cookies = nil
		cp.AppPassword = ""
		if acc.Mailbox != nil {
			mailbox := *acc.Mailbox
			mailbox.Password = ""
			cp.Mailbox = &mailbox
		}
		out = append(out, cp)
	}
	return out
}

// ListSummaries 返回所有帳號的安全摘要,排序為 active → pending → error,
// 同狀態按 name、id 升序。
func (m *Manager) ListSummaries() []Summary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Summary, 0, len(m.accounts))
	for _, acc := range m.accounts {
		out = append(out, acc.Summary())
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := statusRank(out[i].Status), statusRank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// statusRank 返回狀態的排序權重。
func statusRank(status string) int {
	switch status {
	case "active":
		return 0
	case "pending":
		return 1
	default:
		return 2
	}
}

// HMEClient 為指定帳號建立一個新的 HME 用戶端。
// 必須有有效的 Cookie 才能使用 HME 功能。
func (m *Manager) HMEClient(id string, verbose bool) (*hme.Client, error) {
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var snap *Account
	if ok {
		snap = copyAccount(acc)
	}
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("帳號不存在: %s", id)
	}
	if len(snap.Cookies) == 0 {
		return nil, fmt.Errorf("帳號尚未設定 Cookie，無法使用 HME 功能")
	}
	return hme.NewClient(snap.Cookies, snap.Host, snap.Proxy, verbose)
}

// HMEClientWithPassword 為指定帳號建立一個新的 HME 用戶端,使用帳號密碼登入。
// 登入成功後會自動取得 Cookie 並儲存到帳號配置。
func (m *Manager) HMEClientWithPassword(id, password string, otpProvider hme.OTPProvider) (*hme.Client, error) {
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var snap *Account
	if ok {
		snap = copyAccount(acc)
	}
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("帳號不存在: %s", id)
	}

	email := snap.ICloudEmail
	if email == "" {
		email = snap.RealEmail
	}
	if email == "" {
		return nil, fmt.Errorf("帳號尚未設定信箱位址")
	}

	client, err := hme.NewClient(nil, snap.Host, snap.Proxy, true)
	if err != nil {
		return nil, err
	}

	if err := client.Login(email, password, otpProvider); err != nil {
		return nil, err
	}

	// 先儲存 accountLogin 返回的 Cookie，隨後通過 validate 刷新工作階段並再次持久化。
	// 國區與美區都走同一條刷新鏈路，避免只儲存登入階段的臨時 token。
	if err := m.SaveCookies(id, client.Cookies); err != nil {
		return nil, err
	}
	if err := client.ValidateSession(); err != nil {
		// validate 的失敗回應也可能攜帶 Set-Cookie，盡量保留服務端最新狀態。
		_ = m.SaveCookies(id, client.Cookies)
		return nil, err
	}

	// 儲存 validate 刷新後的 Cookie 和帳號狀態。
	m.mu.Lock()
	cur, ok := m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("帳號不存在: %s", id)
	}
	cur.Cookies = cloneCookies(client.Cookies)
	cur.Status = "active"
	cur.LastValidated = time.Now().Format(time.RFC3339)
	cur.LastError = ""
	if info := client.AccountInfo(); info != nil {
		cur.RealEmail = firstNonEmpty(info.AppleID, info.PrimaryEmail)
		if cur.ICloudEmail == "" {
			cur.ICloudEmail = deriveICloudEmail(info)
		}
	}
	saveErr := m.save()
	m.mu.Unlock()
	if saveErr != nil {
		return nil, saveErr
	}

	return client, nil
}

// MailClient 為指定帳號建立 IMAP 郵件用戶端(每次新建, 不走連線池)。
// 需要事先設定 iCloud 信箱和 App 專用密碼。
// 高頻讀信請用 WithMailClient 複用長連線。
func (m *Manager) MailClient(id string) (*mail.Client, error) {
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var snap *Account
	if ok {
		snap = copyAccount(acc)
	}
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("帳號不存在: %s", id)
	}
	if snap.Mailbox != nil && snap.Mailbox.Email != "" && snap.Mailbox.Password != "" {
		return mail.NewClientWithServer(snap.Mailbox.Email, snap.Mailbox.Password, snap.Mailbox.IMAPHost, snap.Mailbox.IMAPPort), nil
	}
	imapEmail := snap.ICloudEmail
	if imapEmail == "" {
		imapEmail = snap.RealEmail
	}
	if !isICloudDomain(imapEmail) {
		return nil, fmt.Errorf("帳號尚未設定 iCloud 信箱（目前：%s）", imapEmail)
	}
	if snap.AppPassword == "" {
		return nil, fmt.Errorf("帳號尚未設定 App 專用密碼")
	}
	return mail.NewClient(imapEmail, snap.AppPassword), nil
}

// WithMailClient 使用連線池中的長連線執行 fn(循序/帳號級)。
// fn 返回後連線保留在池中, 不會 Logout。
func (m *Manager) WithMailClient(id string, fn func(*mail.Client) error) error {
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var mailbox *MailboxConfig
	if ok && acc.Mailbox != nil {
		copy := *acc.Mailbox
		mailbox = &copy
	}
	m.mu.RUnlock()
	if mailbox != nil && mailbox.Email != "" && mailbox.Password != "" {
		mc := mail.NewClientWithServer(mailbox.Email, mailbox.Password, mailbox.IMAPHost, mailbox.IMAPPort)
		if err := mc.Connect(); err != nil {
			return err
		}
		defer mc.Disconnect()
		return fn(mc)
	}
	imapEmail, appPassword, err := m.imapCreds(id)
	if err != nil {
		return err
	}
	if m.imapPool == nil {
		m.imapPool = mail.NewPool()
	}
	return m.imapPool.Do(imapEmail, appPassword, fn)
}

func (m *Manager) imapCreds(id string) (imapEmail, appPassword string, err error) {
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var snap *Account
	if ok {
		snap = copyAccount(acc)
	}
	m.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("帳號不存在: %s", id)
	}
	imapEmail = snap.ICloudEmail
	if imapEmail == "" {
		imapEmail = snap.RealEmail
	}
	if !isICloudDomain(imapEmail) {
		return "", "", fmt.Errorf("帳號尚未設定 iCloud 信箱（目前：%s）", imapEmail)
	}
	if snap.AppPassword == "" {
		return "", "", fmt.Errorf("帳號尚未設定 App 專用密碼")
	}
	return imapEmail, snap.AppPassword, nil
}

// SetMailbox validates and stores an external IMAP mailbox after testing it.
func (m *Manager) SetMailbox(id string, config MailboxConfig) error {
	config.Provider = strings.TrimSpace(config.Provider)
	config.Email = strings.TrimSpace(config.Email)
	config.IMAPHost = strings.TrimSpace(config.IMAPHost)
	if config.Email == "" || config.IMAPHost == "" || config.Password == "" {
		return fmt.Errorf("收件信箱、IMAP 伺服器與授權碼不能為空")
	}
	if strings.Contains(config.IMAPHost, "://") || config.IMAPPort < 1 || config.IMAPPort > 65535 {
		return fmt.Errorf("IMAP 伺服器或連接埠無效")
	}
	m.mu.RLock()
	_, ok := m.accounts[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("帳號不存在: %s", id)
	}
	mc := mail.NewClientWithServer(config.Email, config.Password, config.IMAPHost, config.IMAPPort)
	if err := mc.Connect(); err != nil {
		return err
	}
	_, err := mc.InboxCount()
	mc.Disconnect()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("帳號不存在: %s", id)
	}
	acc.Mailbox = &config
	return m.save()
}

// WebMailClient 為指定帳號建立 Web 郵件用戶端。
// 使用 Cookie 認證，無需 App Password。
func (m *Manager) WebMailClient(id string) (*mail.WebClient, error) {
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var snap *Account
	if ok {
		snap = copyAccount(acc)
	}
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("帳號不存在: %s", id)
	}
	if len(snap.Cookies) == 0 {
		return nil, fmt.Errorf("帳號尚未設定 Cookie，無法讀取郵件")
	}
	// 從 cookies 中取得 dsid
	dsid := ""
	if v, ok := snap.Cookies["X-APPLE-WEBAUTH-USER"]; ok {
		// 解析 "v=1:s=1:d=22789132008" 格式
		parts := strings.Split(v, ":d=")
		if len(parts) == 2 {
			dsid = parts[1]
		}
	}
	return mail.NewWebClient(snap.Cookies, dsid, snap.Host), nil
}

// SetAppPassword 設定 iCloud 信箱和 App 專用密碼,並測試 IMAP 連線。
func (m *Manager) SetAppPassword(id, icloudEmail, appPassword string) error {
	if icloudEmail == "" {
		return fmt.Errorf("iCloud 信箱不能為空")
	}
	if appPassword == "" {
		return fmt.Errorf("App 專用密碼不能為空")
	}

	m.mu.RLock()
	_, ok := m.accounts[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("帳號不存在: %s", id)
	}

	// 測試連線(鎖外)
	mc := mail.NewClient(icloudEmail, appPassword)
	if err := mc.Connect(); err != nil {
		return err
	}
	count, err := mc.InboxCount()
	mc.Disconnect()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("帳號不存在: %s", id)
	}
	acc.ICloudEmail = icloudEmail
	acc.AppPassword = appPassword
	if err := m.save(); err != nil {
		return err
	}
	_ = count
	return nil
}

// SaveCookies 儲存指定帳號的最新 Cookie（HMEClient 操作後刷新的 token）。
// 用於用戶端 validate/操作過程中從 Set-Cookie 取得了新 token 後持久化。
func (m *Manager) SaveCookies(id string, cookies map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[id]
	if !ok {
		return fmt.Errorf("帳號不存在: %s", id)
	}
	acc.Cookies = cloneCookies(cookies)
	return m.save()
}

// UpdateCookies 更新指定帳號的 Cookie,並自動校驗工作階段有效性。
func (m *Manager) UpdateCookies(id string, cookies map[string]string) error {
	if len(cookies) == 0 {
		return fmt.Errorf("cookies 不能為空")
	}
	m.mu.RLock()
	acc, ok := m.accounts[id]
	var snap *Account
	if ok {
		snap = copyAccount(acc)
	}
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("帳號不存在: %s", id)
	}

	// 自動校驗 Cookie 是否有效(鎖外對快照操作)
	snap.Cookies = cookies
	if snap.Host == "" {
		snap.Host = "icloud.com"
	}
	client, err := hme.NewClient(cookies, snap.Host, snap.Proxy, false)
	if err != nil {
		snap.Status = "error"
		snap.LastError = "建立用戶端失敗：" + err.Error()
	} else if err := client.ValidateSession(); err != nil {
		// validate 即使失敗也可能通過 Set-Cookie 刷新部分工作階段狀態。
		snap.Cookies = client.Cookies
		snap.Status = "error"
		snap.LastError = "Cookie 校驗失敗：" + err.Error()
	} else {
		// 顯式儲存 validate 回應刷新的 Cookie，不依賴傳入 map 的引用關係。
		snap.Cookies = client.Cookies
		snap.Status = "active"
		snap.LastValidated = time.Now().Format(time.RFC3339)
		snap.LastError = ""
		if info := client.AccountInfo(); info != nil {
			snap.RealEmail = firstNonEmpty(info.AppleID, info.PrimaryEmail)
			if snap.ICloudEmail == "" {
				snap.ICloudEmail = deriveICloudEmail(info)
			}
		}
	}

	m.mu.Lock()
	cur, ok := m.accounts[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("帳號不存在: %s", id)
	}
	cur.Cookies = snap.Cookies
	cur.Status = snap.Status
	cur.LastValidated = snap.LastValidated
	cur.LastError = snap.LastError
	cur.RealEmail = snap.RealEmail
	if cur.ICloudEmail == "" {
		cur.ICloudEmail = snap.ICloudEmail
	}
	saveErr := m.save()
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return saveErr
}

// ---- 輔助函式 ----

// deriveICloudEmail 從帳號身份推導 iCloud 信箱位址(用於 IMAP 登入)。
//
// 規則:
//  1. primaryEmail 是 @icloud.com/@me.com/@mac.com → 直接用
//  2. appleId 是上述域名 → 直接用
//  3. appleId 是第三方信箱(如 @qq.com) → 取 local part 拼 @icloud.com
func deriveICloudEmail(info *hme.AccountInfo) string {
	primary := strings.TrimSpace(info.PrimaryEmail)
	appleID := strings.TrimSpace(info.AppleID)

	if isICloudDomain(primary) {
		return primary
	}
	if isICloudDomain(appleID) {
		return appleID
	}
	if strings.Contains(appleID, "@") {
		local := strings.SplitN(appleID, "@", 2)[0]
		return local + "@icloud.com"
	}
	return firstNonEmpty(primary, appleID)
}

func isICloudDomain(email string) bool {
	return email != "" && (strings.Contains(email, "@icloud.com") ||
		strings.Contains(email, "@me.com") ||
		strings.Contains(email, "@mac.com"))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
