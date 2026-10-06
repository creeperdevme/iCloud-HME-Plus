// Package mail - iCloud Web 郵件用戶端
//
// 使用 Cookie 認證通過 iCloud Web API 讀取郵件，
// 無需 App Password。基於 mccgateway 服務。
package mail

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/google/uuid"
)

// WebClientBuildNumber 是與瀏覽器一致的 mccgateway 郵件介面構建號。
const WebClientBuildNumber = "2624Build13"

// WebClient 是 iCloud Web 郵件用戶端。
type WebClient struct {
	cookies       map[string]string
	dsid          string
	clientID      string
	mccGatewayURL string
	host          string // "icloud.com" 或 "icloud.com.cn"
	httpc         tls_client.HttpClient
}

// NewWebClient 建立一個 Web 郵件用戶端。
func NewWebClient(cookies map[string]string, dsid, host string) *WebClient {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
	}

	httpc, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

	if host == "" {
		host = "icloud.com"
	}

	c := &WebClient{
		cookies:  cookies,
		dsid:     dsid,
		clientID: uuid.New().String(),
		host:     host,
		httpc:    httpc,
	}

	// 設定 Cookie 到所有相關域名(確保跨域請求能傳遞 Cookie)
	if len(cookies) > 0 {
		suffix := "icloud.com"
		if host == "icloud.com.cn" {
			suffix = "icloud.com.cn"
		}
		domains := []string{
			"https://setup." + suffix,
			"https://www." + suffix,
			"https://p217-mccgateway." + suffix,
			"https://p217-maildomainws." + suffix,
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

	return c
}

// origin 返回當前帳號對應的 Web Origin。
func (c *WebClient) origin() string {
	return "https://www." + c.host
}

// setCommonHeaders 設定與瀏覽器一致的通用請求頭。
func (c *WebClient) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin())
	req.Header.Set("Referer", c.origin()+"/")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
}

// withParams 給 URL 追加 clientBuildNumber / clientId / dsid 查詢參數。
func (c *WebClient) withParams(rawURL string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sclientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		rawURL, sep, WebClientBuildNumber, WebClientBuildNumber, c.clientID, c.dsid)
}

// resolveMccGateway 從 validate 回應中取得 mccgateway URL。
func (c *WebClient) resolveMccGateway() error {
	if c.mccGatewayURL != "" {
		return nil
	}

	setupURL := "https://setup." + c.host + "/setup/ws/1/validate"
	req, err := http.NewRequest("POST", c.withParams(setupURL), nil)
	if err != nil {
		return err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("validate 失敗: HTTP %d - %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed struct {
		Webservices struct {
			Mccgateway struct {
				URL string `json:"url"`
			} `json:"mccgateway"`
		} `json:"webservices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("解析 validate 回應失敗：%w", err)
	}

	mccURL := parsed.Webservices.Mccgateway.URL
	if mccURL == "" {
		return fmt.Errorf("找不到 mccgateway URL，回應：%s", truncate(string(body), 200))
	}
	if !strings.HasPrefix(mccURL, "https://") {
		mccURL = "https://" + mccURL
	}
	// 去掉連接埠號(如 :443)——tls-client 的 cookie jar 按不帶連接埠的 host 儲存 Cookie,
	// 帶連接埠的 URL 會導致 Cookie 無法附加,返回 403。
	if u, err := url.Parse(mccURL); err == nil && u.Host != "" {
		u.Host = u.Hostname()
		mccURL = u.String()
	}
	c.mccGatewayURL = strings.TrimRight(mccURL, "/")
	return nil
}

// threadSearchResp 是 thread/search 介面的回應結構。
type threadSearchResp struct {
	TotalThreadsReturned int `json:"totalThreadsReturned"`
	ThreadList           []struct {
		ThreadID  string   `json:"threadId"`
		Subject   string   `json:"subject"`
		Senders   []string `json:"senders"`
		Preview   string   `json:"preview"`
		Timestamp int64    `json:"timestamp"`
	} `json:"threadList"`
}

// search 執行 thread/search 請求,返回解析後的郵件列表。
func (c *WebClient) search(payload string) ([]Message, error) {
	if err := c.resolveMccGateway(); err != nil {
		return nil, err
	}

	searchURL := c.withParams(c.mccGatewayURL + "/mailws2/v1/thread/search")
	req, err := http.NewRequest("POST", searchURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("取得郵件失敗：HTTP %d - %s", resp.StatusCode, truncate(string(body), 300))
	}
	if strings.Contains(string(body), `"success":false`) {
		return nil, fmt.Errorf("取得郵件失敗：%s", truncate(string(body), 300))
	}

	var result threadSearchResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析郵件回應失敗：%w", err)
	}

	messages := make([]Message, 0, len(result.ThreadList))
	for _, t := range result.ThreadList {
		from := ""
		if len(t.Senders) > 0 {
			from = t.Senders[0]
		}
		date := ""
		if t.Timestamp > 0 {
			date = time.UnixMilli(t.Timestamp).Format(time.RFC3339)
		}
		messages = append(messages, Message{
			ID:      t.ThreadID,
			From:    from,
			Subject: t.Subject,
			Preview: sanitizePreview(t.Preview),
			Date:    date,
		})
	}
	return messages, nil
}

// ListInbox 列出收件匣郵件。
func (c *WebClient) ListInbox(limit int) ([]Message, error) {
	payload := fmt.Sprintf(`{"responseType":"THREAD_DIGEST","includeFolderStatus":true,"maxResults":%d,"sessionHeaders":{"folder":"INBOX","modseq":null,"threadmodseq":null,"condstore":1,"qresync":1,"threadmode":1}}`, limit)
	return c.search(payload)
}

// SearchMails 搜索郵件。query 為空時等價於 ListInbox。
func (c *WebClient) SearchMails(query string, limit int) ([]Message, error) {
	if query == "" {
		return c.ListInbox(limit)
	}
	payload := fmt.Sprintf(`{"responseType":"THREAD_DIGEST","includeFolderStatus":false,"maxResults":%d,"query":%q,"sessionHeaders":{"folder":"INBOX","condstore":1,"qresync":1,"threadmode":1}}`, limit, query)
	return c.search(payload)
}

// FindByAlias 查找發給指定別名的郵件——在本地過濾(Web API 不支援收件人搜索)。
func (c *WebClient) FindByAlias(alias string, limit int) ([]Message, error) {
	// 拉取收件匣全部郵件(最多取 2*limit),本地過濾
	batchSize := limit * 2
	if batchSize < 50 {
		batchSize = 50
	}
	raw, err := c.ListInbox(batchSize)
	if err != nil {
		return nil, err
	}

	// 本地過濾: To/CC/BCC 或主題中包含 alias
	filtered := make([]Message, 0, limit)
	for _, m := range raw {
		if strings.Contains(strings.ToLower(m.Subject), strings.ToLower(alias)) ||
			strings.Contains(strings.ToLower(m.From), strings.ToLower(alias)) ||
			strings.Contains(strings.ToLower(m.To), strings.ToLower(alias)) {
			filtered = append(filtered, m)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
