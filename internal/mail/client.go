// Package mail 實現 iCloud 郵件 IMAP 讀取用戶端。
//
// 通過 Apple 應用專用密碼連線 imap.mail.me.com:993,
// 拉取隱私信箱別名收到的郵件。對應原 Python 項目 icloud_mail.py。
package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"
)

const (
	IMAPServer = "imap.mail.me.com"
	IMAPPort   = 993
)

// Message 是一封郵件的摘要資訊。
type Message struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Preview string `json:"preview"`
}

// FullMessage 是一封郵件的完整內容(含正文)。
type FullMessage struct {
	Message
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

// Client 是 iCloud 郵件 IMAP 用戶端。
type Client struct {
	username string
	password string
	server   string
	port     int
	cli      *client.Client
}

// NewClient 建立 IMAP 用戶端。需在呼叫其它方法前先 Connect。
func NewClient(appleID, appPassword string) *Client {
	return NewClientWithServer(appleID, appPassword, IMAPServer, IMAPPort)
}

// NewClientWithServer creates an IMAP client for a custom server.
func NewClientWithServer(username, password, server string, port int) *Client {
	return &Client{username: username, password: password, server: server, port: port}
}

// Connect 連線並登入 IMAP 伺服器。已連線且存活時直接複用。
func (c *Client) Connect() error {
	if c.cli != nil {
		if err := c.cli.Noop(); err == nil {
			return nil
		}
		c.forceClose()
	}
	addr := fmt.Sprintf("%s:%d", c.server, c.port)
	cli, err := client.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("IMAP 連線失敗：%w", err)
	}
	if err := cli.Login(c.username, c.password); err != nil {
		_ = cli.Logout()
		return fmt.Errorf("IMAP 登入失敗 — 請檢查信箱帳號、授權碼與伺服器位址：%w", err)
	}
	c.cli = cli
	return nil
}

// Ping 探測連線是否仍可用(NOOP)。
func (c *Client) Ping() error {
	if c.cli == nil {
		return fmt.Errorf("尚未連線")
	}
	return c.cli.Noop()
}

// Disconnect 登出並關閉連線。
func (c *Client) Disconnect() {
	if c.cli != nil {
		_ = c.cli.Logout()
		c.cli = nil
	}
}

// forceClose 不發 LOGOUT, 直接掐斷(壞連線/池丟棄時用)。
func (c *Client) forceClose() {
	if c.cli != nil {
		_ = c.cli.Terminate()
		c.cli = nil
	}
}

// InboxCount 返回收件匣郵件總數。
func (c *Client) InboxCount() (int, error) {
	if c.cli == nil {
		return 0, fmt.Errorf("尚未連線")
	}
	mbox, err := c.cli.Select("INBOX", false)
	if err != nil {
		return 0, err
	}
	return int(mbox.Messages), nil
}

// ListInbox 拉取收件匣最近 limit 封郵件摘要。
//
// days 用於過濾只看近 N 天的郵件(0 表示不限制)。
// 返回按時間倒序排列。
func (c *Client) ListInbox(limit int, days int) ([]Message, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("尚未連線")
	}
	if limit <= 0 {
		limit = 50
	}

	mbox, err := c.cli.Select("INBOX", true)
	if err != nil {
		return nil, err
	}
	total := int(mbox.Messages)
	if total == 0 {
		return []Message{}, nil
	}

	// 計算起始序號(只取最近 limit 封)
	from := uint32(1)
	if uint32(limit) < mbox.Messages {
		from = mbox.Messages - uint32(limit) + 1
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)

	// 拉取完整正文,以便填充 Preview(OTP 驗證碼在正文中); PEEK 不標已讀
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{
		imap.FetchUid,
		imap.FetchEnvelope,
		imap.FetchInternalDate,
		section.FetchItem(),
	}

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.Fetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		m := toMessageWithBody(msg)
		// days 過濾
		if days > 0 {
			if t, err := time.Parse(time.RFC1123Z, m.Date); err == nil {
				if time.Since(t) > time.Duration(days)*24*time.Hour {
					continue
				}
			}
		}
		out = append(out, m)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

// FindByRecipient 查找發給指定隱私信箱別名的最近 limit 封郵件(新→舊)。
//
// 先嘗試 IMAP TO 搜索; 失敗則只掃收件匣最近若干封本地過濾。
func (c *Client) FindByRecipient(recipient string, limit int, days int) ([]Message, error) {
	var out []Message
	err := c.ForEachByRecipient(recipient, limit, days, func(m Message) bool {
		out = append(out, m)
		return true // 收滿 limit 為止
	})
	return out, err
}

// ForEachByRecipient 按新→舊遍歷發給 recipient 的最近 limit 封郵件。
// onMsg 返回 false 時立即停止(用於 OTP 命中即返回)。
func (c *Client) ForEachByRecipient(recipient string, limit int, days int, onMsg func(Message) bool) error {
	if c.cli == nil {
		return fmt.Errorf("尚未連線")
	}
	if onMsg == nil {
		return fmt.Errorf("onMsg 不能為空")
	}
	if limit <= 0 {
		limit = 5
	}

	if _, err := c.cli.Select("INBOX", true); err != nil {
		return err
	}

	// 1) 服務端按 To 搜索
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("To", recipient)
	if days > 0 {
		criteria.Since = time.Now().AddDate(0, 0, -days)
	}
	uids, err := c.cli.UidSearch(criteria)
	if err == nil && len(uids) > 0 {
		// UID 升序 → 取最後 limit 個(最新) → 倒序遍歷
		uids = newestUIDs(uids, limit)
		for i := len(uids) - 1; i >= 0; i-- {
			m, ferr := c.fetchOneUID(uids[i])
			if ferr != nil {
				return ferr
			}
			if !onMsg(m) {
				return nil
			}
		}
		return nil
	}

	// 2) fallback: 只掃最近 N 封信封, 命中 To 再拉 body
	return c.forEachRecentMatching(recipient, limit, days, onMsg)
}

// newestUIDs 保留 UID 列表中最新的 limit 個(假定 UID 升序)。
func newestUIDs(uids []uint32, limit int) []uint32 {
	if limit <= 0 || len(uids) <= limit {
		return uids
	}
	return uids[len(uids)-limit:]
}

// forEachRecentMatching 拉取收件匣最近 scan 封(僅 envelope), 本地按 To 過濾後再取 body。
func (c *Client) forEachRecentMatching(recipient string, limit int, days int, onMsg func(Message) bool) error {
	mbox, err := c.cli.Select("INBOX", true)
	if err != nil {
		return err
	}
	total := int(mbox.Messages)
	if total == 0 {
		return nil
	}
	// 只掃最近 scan 封, 避免全箱
	scan := limit * 4
	if scan < 20 {
		scan = 20
	}
	if scan > 80 {
		scan = 80
	}
	if scan > total {
		scan = total
	}
	from := mbox.Messages - uint32(scan) + 1
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)

	// 僅 envelope + date, 不拉 body
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate}
	messages := make(chan *imap.Message, scan)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.Fetch(seqset, items, messages)
	}()

	type cand struct {
		uid  uint32
		date time.Time
		to   string
	}
	var cands []cand
	recipient = strings.ToLower(recipient)
	for msg := range messages {
		if msg == nil || msg.Envelope == nil {
			continue
		}
		to := ""
		if len(msg.Envelope.To) > 0 {
			parts := make([]string, 0, len(msg.Envelope.To))
			for _, a := range msg.Envelope.To {
				parts = append(parts, a.Address())
			}
			to = strings.Join(parts, ", ")
		}
		if !strings.Contains(strings.ToLower(to), recipient) {
			continue
		}
		if days > 0 && !msg.Envelope.Date.IsZero() {
			if time.Since(msg.Envelope.Date) > time.Duration(days)*24*time.Hour {
				continue
			}
		}
		cands = append(cands, cand{uid: msg.Uid, date: msg.Envelope.Date, to: to})
	}
	if err := <-done; err != nil {
		return err
	}
	// 新→舊
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			if cands[j].date.After(cands[i].date) {
				cands[i], cands[j] = cands[j], cands[i]
			}
		}
	}
	n := 0
	for _, cd := range cands {
		if n >= limit {
			break
		}
		m, ferr := c.fetchOneUID(cd.uid)
		if ferr != nil {
			return ferr
		}
		n++
		if !onMsg(m) {
			return nil
		}
	}
	return nil
}

// fetchOneUID 拉取單封郵件(含 body preview), 使用 BODY.PEEK 不標已讀。
func (c *Client) fetchOneUID(uid uint32) (Message, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()
	msg := <-messages
	if err := <-done; err != nil {
		return Message{}, err
	}
	if msg == nil {
		return Message{}, fmt.Errorf("郵件不存在（uid=%d）", uid)
	}
	return toMessageWithBody(msg), nil
}

func (c *Client) fetchByUIDs(uids []uint32, limit int) ([]Message, error) {
	if len(uids) == 0 {
		return []Message{}, nil
	}
	uids = newestUIDs(uids, limit)
	var out []Message
	// 新→舊
	for i := len(uids) - 1; i >= 0; i-- {
		m, err := c.fetchOneUID(uids[i])
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
	return out, nil
}

// GetFull 取得單封郵件的完整內容(含正文)。
func (c *Client) GetFull(uid uint32) (*FullMessage, error) {
	if c.cli == nil {
		return nil, fmt.Errorf("尚未連線")
	}
	if _, err := c.cli.Select("INBOX", true); err != nil {
		return nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate, imap.FetchRFC822}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.cli.UidFetch(seqset, items, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("郵件不存在（uid=%d）", uid)
	}

	full := &FullMessage{Message: toMessage(msg)}
	// 解析正文
	if r := msg.GetBody(&imap.BodySectionName{}); r != nil {
		if em, err := mail.ReadMessage(r); err == nil {
			body, _ := readBody(em)
			full.Body = body
			full.ContentType = em.Header.Get("Content-Type")
		}
	}
	return full, nil
}

// Delete 刪除收件匣中指定 UID 的郵件。
func (c *Client) Delete(uid uint32) error {
	if c.cli == nil {
		return fmt.Errorf("尚未連線")
	}
	if uid == 0 {
		return fmt.Errorf("郵件 UID 無效")
	}
	if _, err := c.cli.Select("INBOX", false); err != nil {
		return err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	if err := c.cli.UidStore(seqset, item, []interface{}{imap.DeletedFlag}, nil); err != nil {
		return err
	}
	return c.cli.Expunge(nil)
}

// ---- 解析工具 ----

func toMessage(msg *imap.Message) Message {
	m := Message{}
	if msg.Uid > 0 {
		m.ID = fmt.Sprintf("%d", msg.Uid)
	}
	if msg.Envelope != nil {
		if len(msg.Envelope.From) > 0 {
			m.From = msg.Envelope.From[0].Address()
		}
		if len(msg.Envelope.To) > 0 {
			addrs := make([]string, 0, len(msg.Envelope.To))
			for _, a := range msg.Envelope.To {
				addrs = append(addrs, a.Address())
			}
			m.To = strings.Join(addrs, ", ")
		}
		m.Subject = decodeHeader(msg.Envelope.Subject)
		if !msg.Envelope.Date.IsZero() {
			m.Date = msg.Envelope.Date.Format(time.RFC3339)
		}
	}
	if m.From != "" {
		m.From = decodeHeader(m.From)
	}
	if m.To != "" {
		m.To = decodeHeader(m.To)
	}
	return m
}

// toMessageWithBody 在 toMessage 基礎上解析正文填充 Preview(供 OTP 提取)。
func toMessageWithBody(msg *imap.Message) Message {
	m := toMessage(msg)
	// Fetch 可能用 BODY[] 或 BODY.PEEK[], 兩種 section 都試
	for _, section := range []*imap.BodySectionName{{Peek: true}, {}} {
		r := msg.GetBody(section)
		if r == nil {
			continue
		}
		em, err := mail.ReadMessage(r)
		if err != nil {
			continue
		}
		body, err := readBody(em)
		if err != nil {
			continue
		}
		m.Preview = strings.TrimSpace(body)
		break
	}
	return m
}

// decodeHeader 解碼 RFC 2047 編碼的郵件頭(如 =?UTF-8?B?xxx?=)。
func decodeHeader(s string) string {
	if s == "" {
		return ""
	}
	dec := mime.WordDecoder{CharsetReader: charset.Reader}
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

// readBody 讀取郵件正文,優先 text/plain,其次從 HTML 提取純文本。
//
// 會實際走訪 MIME 結構:multipart/* 逐層遞迴、quoted-printable 與 base64 解碼、
// 非 UTF-8 字集轉換。
//
// 先前的版本只看最外層的 Content-Type,於是 multipart 郵件(幾乎所有真實信件都是)
// 被當成純文字處理:整份 MIME 原始碼(含 boundary、各部分標頭、HTML 與 CSS)直接
// 送進 sanitizePlainPreview,被誤判成 CSS 而清空,前端就顯示「(無內文)」。
func readBody(msg *mail.Message) (string, error) {
	plain, html := extractText(msg.Header, msg.Body)
	if text := strings.TrimSpace(plain); text != "" {
		return sanitizePlainPreview(text), nil
	}
	return sanitizePreview(html), nil
}

// extractText 遞迴走訪郵件結構,分別取出純文字與 HTML 內容。
//
// multipart/alternative 的各部分由簡到繁排列,因此第一個非空的 text/plain 即為
// 純文字版本;沒有純文字版本時才退回 HTML(由呼叫端處理)。
func extractText(h mail.Header, body io.Reader) (plain, html string) {
	mediaType, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		// Content-Type 缺失或無法解析時,保守當成純文字(與舊行為一致)
		mediaType, params = "text/plain", nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "", ""
		}
		reader := multipart.NewReader(body, boundary)
		for {
			part, err := reader.NextPart()
			if err != nil {
				// io.EOF 代表正常結束;其他錯誤也無從補救,已取到的部分照用
				break
			}
			partPlain, partHTML := extractText(mail.Header(part.Header), part)
			_ = part.Close()
			if plain == "" {
				plain = partPlain
			}
			if html == "" {
				html = partHTML
			}
		}
		return plain, html
	}

	raw, err := io.ReadAll(body)
	if err != nil {
		return "", ""
	}
	text := decodeCharset(params["charset"], decodeTransferEncoding(h.Get("Content-Transfer-Encoding"), raw))

	switch mediaType {
	case "text/plain":
		return text, ""
	case "text/html":
		return "", text
	default:
		// text/calendar、application/* 與各種附件都不算正文
		return "", ""
	}
}

// decodeTransferEncoding 依 Content-Transfer-Encoding 解碼郵件內容。
func decodeTransferEncoding(encoding string, raw []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		if out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw))); err == nil {
			return out
		}
	case "base64":
		if out, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(raw))); err == nil {
			return out
		}
	}
	return raw
}

// decodeCharset 把非 UTF-8 的內容轉成 UTF-8;無法辨識時原文回傳。
func decodeCharset(name string, raw []byte) string {
	label := strings.ToLower(strings.TrimSpace(name))
	switch label {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return string(raw)
	}
	reader, err := charset.Reader(label, bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		return string(raw)
	}
	return string(out)
}
