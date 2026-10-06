// Package hme 實現了 iCloud Hide My Email 協議用戶端。
//
// 基於 Cookie 工作階段,通過 tls-client 偽裝 Chrome TLS 指紋規避 iCloud 風控。
// 對應原 Python 項目 icloud_hme.py 的 ICloudHME 類。
package hme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	// ClientBuildNumber 是 iCloud Web 用戶端構建號,從瀏覽器抓包取得。
	// maildomainws (HME 別名管理) 專用。
	ClientBuildNumber = "2624Build22"
	// ClientMasteringNumber 是 iCloud Web 用戶端主版本號。
	ClientMasteringNumber = "2624Build22"
	// DefaultBuildNumber 用於 validate 和 mccgateway (郵件) 等非 HME 端點。
	DefaultBuildNumber = "2624Build13"
	// RequestTimeout 單次請求超時。
	RequestTimeout = 15 * time.Second
	// MaxRetries 最大重試次數。
	MaxRetries = 3
)

var retryDelays = []time.Duration{
	1 * time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
}

// AccountInfo 是從 /validate 回應中提取的帳號身份資訊。
type AccountInfo struct {
	DSID             string `json:"dsid"`
	AppleID          string `json:"appleId"`
	PrimaryEmail     string `json:"primaryEmail"`
	FullName         string `json:"fullName"`
	IsManagedAppleID bool   `json:"isManagedAppleId"`
}

// Alias 是一個 Hide My Email 隱私信箱別名。
type Alias struct {
	Email       string `json:"email"`
	AnonymousID string `json:"anonymousId"`
	Label       string `json:"label"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

// Client 是 iCloud Hide My Email 用戶端。
//
// 一個 Client 對應一個 iCloud 帳號。通過傳入的 Cookie 維持工作階段,
// 首次呼叫業務方法時會自動觸發 ValidateSession 解析 HME 服務端點。
type Client struct {
	Cookies     map[string]string
	Host        string // "icloud.com" 或 "icloud.com.cn"
	Proxy       string // HTTP/SOCKS5 代理
	Username    string // iCloud 帳號 (用於登入)
	Password    string // iCloud 密碼 (用於登入)
	Verbose     bool
	httpc       tls_client.HttpClient
	setupURL    string
	serviceURL  string
	dsid        string // 從 validate 回應提取
	clientID    string // UUID,每次工作階段產生
	accountInfo *AccountInfo
}

// NewClient 建立一個新的 HME 用戶端,底層使用 Chrome TLS 指紋。
//
// proxy 支援格式:
//   - HTTP:  "http://user:pass@host:port"
//   - SOCKS5: "socks5://user:pass@host:port"
func NewClient(cookies map[string]string, host, proxy string, verbose bool) (*Client, error) {
	if host == "" {
		host = "icloud.com"
	}
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
	}

	// 新增代理支援
	if proxy != "" {
		options = append(options, tls_client.WithProxyUrl(proxy))
	}

	httpc, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}

	c := &Client{
		Cookies:  cookies,
		Host:     normalizeHost(host),
		Proxy:    proxy,
		Verbose:  verbose,
		httpc:    httpc,
		clientID: uuid.New().String(),
	}

	// 把傳入的 Cookie 灌入 jar,後續請求自動攜帶。
	if len(cookies) > 0 {
		// 設定 Cookie 到所有可能的域名
		domains := []string{
			"https://www.icloud.com",
			"https://www.icloud.com.cn",
			"https://setup.icloud.com",
			"https://setup.icloud.com.cn",
			"https://" + c.Host,
		}

		// 新增 serviceURL 的域名（如果已知）
		if c.serviceURL != "" {
			if u, err := url.Parse(c.serviceURL); err == nil {
				domains = append(domains, u.Scheme+"://"+u.Host)
			}
		}

		for _, domain := range domains {
			u, _ := url.Parse(domain)
			httpCookies := make([]*http.Cookie, 0, len(cookies))
			for k, v := range cookies {
				httpCookies = append(httpCookies, &http.Cookie{
					Name:  k,
					Value: v,
					Path:  "/",
				})
			}
			jar.SetCookies(u, httpCookies)
		}
	}
	return c, nil
}

func normalizeHost(host string) string {
	h := strings.TrimSpace(strings.ToLower(host))
	if u, err := url.Parse(h); err == nil && u.Hostname() != "" {
		h = u.Hostname()
	} else if !strings.Contains(h, "://") {
		if u, err := url.Parse("https://" + h); err == nil && u.Hostname() != "" {
			h = u.Hostname()
		}
	}
	if strings.HasSuffix(h, ".icloud.com.cn") || h == "icloud.com.cn" {
		return "icloud.com.cn"
	}
	return "icloud.com"
}

// SetupURL 返回 iCloud setup 端點。
func (c *Client) SetupURL() string {
	if c.setupURL == "" {
		suffix := "setup.icloud.com"
		if c.Host == "icloud.com.cn" {
			suffix = "setup.icloud.com.cn"
		}
		c.setupURL = "https://" + suffix + "/setup/ws/1"
	}
	return c.setupURL
}

// Origin 返回 Web Origin。
func (c *Client) Origin() string {
	return "https://www." + c.Host
}

func (c *Client) log(format string, args ...any) {
	if c.Verbose {
		fmt.Printf("  [iCloud] %s\n", fmt.Sprintf(format, args...))
	}
}

// buildURL 給 URL 追加 clientBuildNumber / clientMasteringNumber / clientId / dsid 查詢參數,
// 這是 iCloud Web API 的強制要求。
func (c *Client) buildURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	// setup.icloud.com (validate) 和 mccgateway 用 DefaultBuildNumber,maildomainws 用 ClientBuildNumber
	host := parsed.Hostname()
	if strings.Contains(host, "maildomainws") {
		q.Set("clientBuildNumber", ClientBuildNumber)
		q.Set("clientMasteringNumber", ClientMasteringNumber)
	} else {
		q.Set("clientBuildNumber", DefaultBuildNumber)
		q.Set("clientMasteringNumber", DefaultBuildNumber)
	}
	if c.clientID != "" {
		q.Set("clientId", c.clientID)
	}
	if c.dsid != "" {
		q.Set("dsid", c.dsid)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// requestOrigin 根據實際請求端點選擇 Origin。
// Apple 可能把國區帳號路由到全球服務，Origin 必須跟隨目標域名而不是帳號配置。
func requestOrigin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		host := strings.ToLower(u.Hostname())
		if host == "icloud.com.cn" || strings.HasSuffix(host, ".icloud.com.cn") {
			return "https://www.icloud.com.cn"
		}
		if host == "icloud.com" || strings.HasSuffix(host, ".icloud.com") {
			return "https://www.icloud.com"
		}
	}
	return "https://www.icloud.com"
}

// request 執行帶重試的 HTTP 請求,返回回應體字串。
func (c *Client) request(method, rawURL string, body any, timeout time.Duration, maxAttempts int) (string, error) {
	if timeout == 0 {
		timeout = RequestTimeout
	}
	if maxAttempts == 0 {
		maxAttempts = MaxRetries
	}
	fullURL := c.buildURL(rawURL)

	hostName := ""
	if u, err := url.Parse(rawURL); err == nil {
		hostName = u.Hostname()
	}
	contentType := "application/json"
	acceptType := "application/json, text/plain, */*"
	if strings.Contains(hostName, "maildomainws") {
		contentType = "text/plain"
		acceptType = "*/*"
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var reqBody io.Reader
		if body != nil {
			buf, err := json.Marshal(body)
			if err != nil {
				return "", err
			}
			reqBody = bytes.NewReader(buf)
		}

		req, err := http.NewRequest(method, fullURL, reqBody)
		if err != nil {
			return "", err
		}
		origin := requestOrigin(rawURL)
		req.Header.Set("Origin", origin)
		req.Header.Set("Referer", origin+"/")
		req.Header.Set("Accept", acceptType)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-site")
		req.Header.Set("sec-ch-ua", `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
		req.Header.Set("sec-ch-ua-mobile", "?0")
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")

		// 手動新增 Cookie 頭（確保跨域也能傳遞）
		// 瀏覽器發送的 Cookie 值帶雙引號,iCloud 嚴格匹配
		if len(c.Cookies) > 0 {
			cookieParts := make([]string, 0, len(c.Cookies))
			for k, v := range c.Cookies {
				if strings.HasPrefix(v, `"`) {
					cookieParts = append(cookieParts, k+"="+v)
				} else {
					cookieParts = append(cookieParts, k+`="`+v+`"`)
				}
			}
			cookieHeader := strings.Join(cookieParts, "; ")
			req.Header.Set("Cookie", cookieHeader)
			if c.Verbose {
				c.log(">>> URL: %s", fullURL)
				c.log(">>> Cookie: %s", cookieHeader[:min(200, len(cookieHeader))])
				for k, vv := range req.Header {
					for _, v := range vv {
						c.log(">>> %s: %s", k, v[:min(100, len(v))])
					}
				}
			}
		}

		resp, err := c.httpc.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("連線失敗：%w", err)
			if attempt < maxAttempts {
				c.sleepRetry(attempt)
				continue
			}
			return "", lastErr
		}

		text, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// 從 Set-Cookie 回應頭更新 Cookie（模擬瀏覽器行為,iCloud 會刷新 token）
		for _, sc := range resp.Cookies() {
			if sc.Name != "" && sc.Value != "" {
				c.Cookies[sc.Name] = sc.Value
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snippet := string(text)
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
			// 401/403 說明 Cookie 失效,不重試直接返回。
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				return "", lastErr
			}
			if attempt < maxAttempts {
				c.sleepRetry(attempt)
				continue
			}
			return "", lastErr
		}

		return string(text), nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("未知錯誤")
}

func (c *Client) sleepRetry(attempt int) {
	idx := attempt - 1
	if idx >= len(retryDelays) {
		idx = len(retryDelays) - 1
	}
	time.Sleep(retryDelays[idx])
}

// validationURLs 返回工作階段校驗端點。
// 國區優先使用本地區域端點，並回退到全球端點以兼容 Apple 的路由調整。
func (c *Client) validationURLs() []string {
	primary := c.SetupURL() + "/validate"
	if c.Host != "icloud.com.cn" {
		return []string{primary}
	}
	global := "https://setup.icloud.com/setup/ws/1/validate"
	if primary == global {
		return []string{primary}
	}
	return []string{primary, global}
}

// ValidateSession 校驗 iCloud 工作階段,解析 HME 服務端點和帳號身份。
//
// 必須在呼叫 ListAliases / Generate / Reserve / Delete 之前完成。
// 失敗通常意味著 Cookie 過期或未訂閱 iCloud+。
func (c *Client) ValidateSession() error {
	c.log("校驗 iCloud 工作階段…")
	c.log("使用的 Cookie 數量：%d", len(c.Cookies))
	if len(c.Cookies) > 0 {
		for k := range c.Cookies {
			c.log("Cookie: %s", k)
		}
	}

	var body string
	var err error
	validationURLs := c.validationURLs()
	for i, validationURL := range validationURLs {
		var candidate string
		candidate, err = c.request("POST", validationURL, nil, 20*time.Second, MaxRetries)
		if err == nil && !gjson.Valid(candidate) {
			err = fmt.Errorf("invalid JSON response")
		}
		if err == nil && gjson.Get(candidate, "webservices.premiummailsettings.url").String() == "" {
			err = fmt.Errorf("validate 回應缺少 Hide My Email 服務端點")
		}
		if err == nil {
			body = candidate
			break
		}
		if i < len(validationURLs)-1 {
			c.log("區域 validate 失敗，改用全球端點：%v", err)
		}
	}
	if err != nil {
		c.log("校驗失敗：%v", err)
		return err
	}
	data := gjson.Parse(body)
	serviceURL := data.Get("webservices.premiummailsettings.url").String()
	c.serviceURL = strings.TrimRight(serviceURL, "/")
	// 剝離 :443 連接埠——tls-client cookie jar 按無連接埠 host 儲存 cookie,帶連接埠會丟失 cookie → 401
	if strings.HasSuffix(c.serviceURL, ":443") {
		c.serviceURL = strings.TrimSuffix(c.serviceURL, ":443")
	}

	// 取得 serviceURL 後，再次設定 Cookie 到該域名
	if len(c.Cookies) > 0 {
		u, _ := url.Parse(c.serviceURL)
		httpCookies := make([]*http.Cookie, 0, len(c.Cookies))
		for k, v := range c.Cookies {
			httpCookies = append(httpCookies, &http.Cookie{
				Name:  k,
				Value: v,
				Path:  "/",
			})
		}
		c.httpc.GetCookies(u) // 觸發 cookie jar 初始化
		// 注意：需要手動設定 cookie，但 tls-client 的 CookieJar 不支援直接設定
		// 我們需要在請求時手動新增 Cookie 頭
	}

	dsInfo := data.Get("dsInfo")
	c.dsid = dsInfo.Get("dsid").String()
	info := &AccountInfo{
		DSID:             c.dsid,
		AppleID:          firstNonEmpty(dsInfo.Get("appleId").String(), dsInfo.Get("primaryEmail").String(), dsInfo.Get("appleIdEmail").String()),
		PrimaryEmail:     firstNonEmpty(dsInfo.Get("primaryEmail").String(), dsInfo.Get("appleId").String()),
		FullName:         firstNonEmpty(dsInfo.Get("fullName").String(), dsInfo.Get("name").String()),
		IsManagedAppleID: dsInfo.Get("isManagedAppleId").Bool(),
	}
	if info.AppleID == "" {
		for _, name := range []string{"aosappleid", "appleId", "dsid"} {
			if v, ok := c.Cookies[name]; ok && v != "" {
				info.AppleID = v
				break
			}
		}
	}
	c.accountInfo = info
	c.log("工作階段有效 → %s", nonEmpty(info.AppleID, "未知帳號"))
	return nil
}

// AccountInfo 返回已校驗的帳號身份(校驗前為 nil)。
func (c *Client) AccountInfo() *AccountInfo { return c.accountInfo }

func (c *Client) resolveService() error {
	if c.serviceURL == "" {
		return c.ValidateSession()
	}
	return nil
}

// ListAliases 列出當前帳號所有 Hide My Email 別名。
func (c *Client) ListAliases() ([]Alias, error) {
	if err := c.resolveService(); err != nil {
		return nil, err
	}
	c.log("取得別名列表…")
	body, err := c.request("GET", c.serviceURL+"/v2/hme/list", nil, 0, MaxRetries)
	if err != nil {
		return nil, err
	}
	aliases := parseAliasList(body)
	c.log("共 %d 個別名", len(aliases))
	return aliases, nil
}

// Generate 產生一個候選別名(尚未保留,需再呼叫 Reserve)。
func (c *Client) Generate() (string, error) {
	if err := c.resolveService(); err != nil {
		return "", err
	}
	c.log("產生候選別名…")
	body, err := c.request("POST", c.serviceURL+"/v1/hme/generate", map[string]string{"langCode": "en-us"}, 0, 2)
	if err != nil {
		return "", err
	}
	parsed := gjson.Parse(body)
	if !parsed.Get("success").Bool() {
		errMsg := parsed.Get("error.errorMessage").String()
		return "", fmt.Errorf("產生失敗：%s", nonEmpty(errMsg, "unknown"))
	}
	hme := parsed.Get("result.hme").String()
	if hme == "" {
		// 某些回應把 hme 包在嵌套物件裡
		hme = parsed.Get("result.hme.hme").String()
		if hme == "" {
			hme = parsed.Get("result.hme.email").String()
		}
	}
	c.log("候選：%s", hme)
	return hme, nil
}

// reserveResult 是 /v1/hme/reserve 解析後的結果。
type reserveResult struct {
	// Alias 是最終生效的信箱位址。
	Alias string
	// AnonymousID 是刪除/停用別名時要用的識別碼；上游沒回傳時為空字串。
	AnonymousID string
}

// Reserve 保留/確認候選別名,使其正式生效。
func (c *Client) Reserve(hme, label string) (string, error) {
	r, err := c.reserve(hme, label)
	return r.Alias, err
}

// reserve 與 Reserve 相同,但額外取回 anonymousId。
//
// anonymousId 是之後刪除別名唯一的識別碼,某些上游回應不會帶,因此這裡同時嘗試
// 多種欄位名稱;呼叫端若拿到空字串,應自行用 ListAliases 以信箱反查。
func (c *Client) reserve(hme, label string) (reserveResult, error) {
	if err := c.resolveService(); err != nil {
		return reserveResult{}, err
	}
	if label == "" {
		label = "Created " + time.Now().Format("2006-01-02 15:04")
	}
	c.log("保留別名 %s …", hme)
	payload := map[string]string{
		"hme":   hme,
		"label": label,
		"note":  "Created by icloud_hme tool",
	}
	body, err := c.request("POST", c.serviceURL+"/v1/hme/reserve", payload, 0, 2)
	if err != nil {
		return reserveResult{}, err
	}
	parsed := gjson.Parse(body)
	if !parsed.Get("success").Bool() {
		errMsg := parsed.Get("error.errorMessage").String()
		return reserveResult{}, fmt.Errorf("保留失敗：%s", nonEmpty(errMsg, "unknown"))
	}
	alias := hme
	resultHme := parsed.Get("result.hme")
	if resultHme.IsObject() {
		if v := resultHme.Get("hme").String(); v != "" {
			alias = v
		}
	}
	anonymousID := firstNonEmpty(
		resultHme.Get("anonymousId").String(),
		resultHme.Get("anonymousID").String(),
		resultHme.Get("id").String(),
		resultHme.Get("metaData.anonymousId").String(),
		parsed.Get("result.anonymousId").String(),
	)
	c.log("已保留：%s", alias)
	return reserveResult{Alias: alias, AnonymousID: anonymousID}, nil
}

// CreateResult 是 CreateAlias 的返回結果。
type CreateResult struct {
	Email     string `json:"email"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
	// AnonymousID 用於之後刪除/停用這個別名；上游未回傳時會用別名列表反查。
	AnonymousID string `json:"anonymous_id,omitempty"`
}

// CreateAlias 一步完成「產生 + 保留」,建立一個新別名。
//
// 由於 generate / reserve 偶發失敗,內部會重試 maxRetries 次,
// 每次重試會重置 serviceURL 強制重新校驗工作階段。
func (c *Client) CreateAlias(label string, maxRetries int) (*CreateResult, error) {
	if maxRetries <= 0 {
		maxRetries = 5
	}
	var lastErr string
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			c.serviceURL = ""
			c.setupURL = ""
			c.log("重試 %d/%d …", attempt+1, maxRetries)
		}
		hme, err := c.Generate()
		if err != nil {
			lastErr = "generate 失敗：" + err.Error()
			c.log("%s", lastErr)
			if attempt < maxRetries-1 {
				time.Sleep(time.Second)
				continue
			}
			break
		}
		reserved, err := c.reserve(hme, label)
		if err != nil {
			lastErr = err.Error()
			c.log("reserve 失敗：%s", lastErr)
			if attempt < maxRetries-1 {
				time.Sleep(time.Second)
				continue
			}
			break
		}
		anonymousID := reserved.AnonymousID
		if anonymousID == "" {
			// 上游沒有在 reserve 回應帶 anonymousId 時，用別名列表反查。
			// 拿不到 ID 就無法在 24 小時後自動刪除，因此值得多花一次請求。
			if resolved, lookupErr := c.lookupAnonymousID(reserved.Alias); lookupErr == nil {
				anonymousID = resolved
			} else {
				c.log("反查 anonymousId 失敗：%v", lookupErr)
			}
		}
		return &CreateResult{
			Email:       reserved.Alias,
			Label:       label,
			CreatedAt:   time.Now().Format(time.RFC3339),
			AnonymousID: anonymousID,
		}, nil
	}
	if lastErr != "" {
		return nil, fmt.Errorf("建立別名失敗：%s", lastErr)
	}
	return nil, fmt.Errorf("建立別名失敗，已重試 %d 次", maxRetries)
}

// lookupAnonymousID 用信箱位址在別名列表中反查 anonymousId。
func (c *Client) lookupAnonymousID(email string) (string, error) {
	aliases, err := c.ListAliases()
	if err != nil {
		return "", err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	for _, a := range aliases {
		if strings.ToLower(a.Email) == target && a.AnonymousID != "" {
			return a.AnonymousID, nil
		}
	}
	return "", fmt.Errorf("別名列表中找不到 %s", email)
}

// DeactivateHME 停用別名(可恢復)。
func (c *Client) DeactivateHME(anonymousID string) (bool, error) {
	if err := c.resolveService(); err != nil {
		return false, err
	}
	c.log("停用 %s …", anonymousID)
	payload := map[string]string{"anonymousId": anonymousID}
	body, err := c.request("POST", c.serviceURL+"/v1/hme/deactivate", payload, 0, 2)
	if err != nil {
		return false, err
	}
	return gjson.Get(body, "success").Bool(), nil
}

// ReactivateHME 激活已停用的別名。
func (c *Client) ReactivateHME(anonymousID string) (bool, error) {
	if err := c.resolveService(); err != nil {
		return false, err
	}
	c.log("啟用 %s …", anonymousID)
	payload := map[string]string{"anonymousId": anonymousID}
	body, err := c.request("POST", c.serviceURL+"/v1/hme/reactivate", payload, 0, 2)
	if err != nil {
		return false, err
	}
	return gjson.Get(body, "success").Bool(), nil
}

// Delete 刪除別名。若直接刪除失敗會先停用再刪。
func (c *Client) Delete(anonymousID string) error {
	if err := c.resolveService(); err != nil {
		return err
	}
	c.log("刪除 %s …", anonymousID)
	payload := map[string]string{"anonymousId": anonymousID}
	doDelete := func() (string, error) {
		return c.request("POST", c.serviceURL+"/v1/hme/delete", payload, 0, 2)
	}
	body, err := doDelete()
	if err != nil || !gjson.Get(body, "success").Bool() {
		c.log("直接刪除失敗,嘗試先停用...")
		_, _ = c.request("POST", c.serviceURL+"/v1/hme/deactivate", payload, 0, 2)
		body, err = doDelete()
		if err != nil {
			return err
		}
		if !gjson.Get(body, "success").Bool() {
			return fmt.Errorf("%s", gjson.Get(body, "error.errorMessage").String())
		}
	}
	c.log("已刪除")
	return nil
}

// ---- 別名列表解析 (對應 ICloudHME._parse_alias_list) ----

// parseAliasList 解析 iCloud 返回的別名列表 JSON。
// 容錯:優先取 result.hmeEmails,找不到則遞迴查找第一個物件陣列。
func parseAliasList(body string) []Alias {
	if !gjson.Valid(body) {
		return []Alias{}
	}
	root := gjson.Parse(body)

	arr := root.Get("result.hmeEmails")
	if !arr.IsArray() {
		arr = findFirstDictArray(root)
	}
	if !arr.IsArray() {
		return []Alias{}
	}

	var aliases []Alias
	arr.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() {
			return true
		}
		meta := item.Get("metaData")
		email := strings.TrimSpace(strings.ToLower(firstNonEmpty(
			item.Get("hme").String(),
			item.Get("email").String(),
			item.Get("alias").String(),
			item.Get("address").String(),
			meta.Get("hme").String(),
		)))
		if email == "" || !strings.Contains(email, "@") {
			return true
		}
		state := strings.ToLower(firstNonEmpty(item.Get("state").String(), item.Get("status").String()))
		active := state != "inactive" && state != "deleted"
		if item.Get("active").Exists() {
			active = item.Get("active").Bool() && active
		}
		if item.Get("isActive").Exists() {
			active = item.Get("isActive").Bool() && active
		}
		aliases = append(aliases, Alias{
			Email:       email,
			AnonymousID: firstNonEmpty(item.Get("anonymousId").String(), item.Get("id").String()),
			Label:       firstNonEmpty(item.Get("label").String(), meta.Get("label").String()),
			Active:      active,
			CreatedAt:   firstNonEmpty(item.Get("createTimestamp").String(), item.Get("createdAt").String()),
		})
		return true
	})

	// 活躍的排前面,再按信箱字母序。
	sort.SliceStable(aliases, func(i, j int) bool {
		if aliases[i].Active != aliases[j].Active {
			return aliases[i].Active
		}
		return aliases[i].Email < aliases[j].Email
	})
	return aliases
}

// findFirstDictArray 遞迴查找第一個「物件陣列」。
func findFirstDictArray(v gjson.Result) gjson.Result {
	if v.IsArray() {
		if len(v.Array()) > 0 && v.Array()[0].IsObject() {
			return v
		}
	}
	if v.IsObject() {
		var found gjson.Result
		v.ForEach(func(_, val gjson.Result) bool {
			if r := findFirstDictArray(val); r.IsArray() && len(r.Array()) > 0 {
				found = r
				return false
			}
			return true
		})
		return found
	}
	return gjson.Result{}
}

// ---- 小工具 ----

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
