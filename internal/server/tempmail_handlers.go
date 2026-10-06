// Package server - 隨機信箱（臨時別名）的 HTTP 介面與到期清理。
package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/tempmail"
)

// DefaultTempTTL 是隨機信箱預設的自動刪除時間。
const DefaultTempTTL = 24 * time.Hour

// tempSweepInterval 是到期清理的檢查間隔。
const tempSweepInterval = time.Minute

// maxTempDeleteAttempts 是自動刪除連續失敗幾次後放棄追蹤。
//
// 上游若已經把別名刪掉（例如使用者在別名頁面手動刪除），這裡會一直失敗；
// 沒有上限的話記錄會永遠卡在清單裡。
const maxTempDeleteAttempts = 10

// tempLabelWords 用於組出好辨識的隨機標籤。
var tempLabelWords = []string{
	"amber", "azure", "breeze", "cedar", "coral", "crystal", "dawn", "ember",
	"fern", "frost", "glow", "harbor", "ivory", "jade", "lagoon", "lumen",
	"maple", "meadow", "mist", "nimbus", "onyx", "opal", "pebble", "quartz",
	"raven", "reef", "sable", "sage", "slate", "spruce", "tide", "willow",
}

// randomTempLabel 產生隨機標籤，例如 temp-jade-reef-4821。
func randomTempLabel() string {
	pick := func(n int) int {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return int(time.Now().UnixNano() % int64(n))
		}
		return int(v.Int64())
	}
	return fmt.Sprintf("temp-%s-%s-%04d",
		tempLabelWords[pick(len(tempLabelWords))],
		tempLabelWords[pick(len(tempLabelWords))],
		pick(10000),
	)
}

// tempTTL 回傳設定的自動刪除時間，未設定時用預設值。
func (s *Server) tempTTL() time.Duration {
	if s.cfg.TempTTL > 0 {
		return s.cfg.TempTTL
	}
	return DefaultTempTTL
}

// pickTempAccount 選擇要建立隨機信箱的帳號。
//
// 建立 HME 別名必須有有效的 Cookie（App 專用密碼只能拿來登入換 Cookie，本身
// 不足以呼叫 HME 介面），因此這裡只挑有 Cookie 的帳號——否則會選到一個註定
// 失敗的帳號，而忽略另一個其實可用的帳號。
//
// 優先使用呼叫端指定的帳號；未指定時挑第一個有 Cookie 的帳號。
func (s *Server) pickTempAccount(requested string) (string, error) {
	accounts := s.be.ListAccounts()
	if requested != "" {
		for _, a := range accounts {
			if a.ID == requested {
				if !a.HasCookies {
					return "", &BackendError{
						Status: http.StatusBadRequest,
						Code:   "VALIDATION_ERROR",
						Message: fmt.Sprintf(
							"帳號「%s」尚未設定 Cookie，無法建立隨機信箱；請先在「帳號管理」登入該帳號或填入 Cookie",
							a.Name),
					}
				}
				return a.ID, nil
			}
		}
		return "", &BackendError{Status: http.StatusNotFound, Code: "ACCOUNT_NOT_FOUND", Message: "找不到指定的帳號"}
	}
	for _, a := range accounts {
		if a.HasCookies {
			return a.ID, nil
		}
	}
	return "", &BackendError{
		Status:  http.StatusBadRequest,
		Code:    "VALIDATION_ERROR",
		Message: "還沒有可用來建立隨機信箱的帳號，請先在「帳號管理」新增並登入 iCloud 帳號",
	}
}

// createTempMailboxHandler 建立一個隨機信箱。
//
// 別名由 iCloud 隨機產生，這裡只負責給一個好辨識的隨機標籤。
func (s *Server) createTempMailboxHandler(c *gin.Context) {
	var body struct {
		AccountID string `json:"account_id"`
	}
	// 允許空 body。
	_ = c.ShouldBindJSON(&body)

	accountID, err := s.pickTempAccount(strings.TrimSpace(body.AccountID))
	if err != nil {
		backendFail(c, err)
		return
	}

	label := randomTempLabel()
	created, err := s.be.CreateAlias(accountID, label)
	if err != nil {
		backendFail(c, err)
		return
	}
	if created.AnonymousID == "" {
		failCode(c, http.StatusBadGateway, "UPSTREAM_FAILURE",
			"iCloud 沒有回傳別名識別碼，無法追蹤這個隨機信箱的自動刪除")
		return
	}

	now := s.now()
	mb := tempmail.Mailbox{
		ID:        created.AnonymousID,
		Email:     created.Email,
		AccountID: accountID,
		Label:     label,
		CreatedAt: now,
		ExpiresAt: now.Add(s.tempTTL()),
	}
	if err := s.temp.Add(mb); err != nil {
		// 別名已經在上游建立成功，這裡失敗只影響追蹤，仍回報建立結果。
		log.Printf("記錄隨機信箱失敗 id=%s：%v", mb.ID, err)
	}
	ok(c, mb)
}

// listTempMailboxesHandler 列出追蹤中的隨機信箱。
func (s *Server) listTempMailboxesHandler(c *gin.Context) {
	items := s.temp.List()
	if items == nil {
		items = []tempmail.Mailbox{}
	}
	ok(c, gin.H{"count": len(items), "mailboxes": items, "ttl_seconds": int(s.tempTTL().Seconds())})
}

// keepTempMailboxHandler 切換是否自動刪除。
func (s *Server) keepTempMailboxHandler(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Keep *bool `json:"keep"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Keep == nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "缺少 keep 參數")
		return
	}
	mb, err := s.temp.SetKeep(id, *body.Keep, s.tempTTL(), s.now())
	if err != nil {
		failCode(c, http.StatusNotFound, "NOT_FOUND", "找不到這個隨機信箱")
		return
	}
	ok(c, mb)
}

// deleteTempMailboxHandler 立刻刪除隨機信箱。
//
// 上游刪除失敗時仍然會停止追蹤：使用者的意圖是「不要再看到它」，
// 而且如果在別名頁面已經刪掉了，這裡必然失敗。
func (s *Server) deleteTempMailboxHandler(c *gin.Context) {
	id := c.Param("id")
	mb, found := s.temp.Get(id)
	if !found {
		failCode(c, http.StatusNotFound, "NOT_FOUND", "找不到這個隨機信箱")
		return
	}
	upstreamErr := s.be.DeleteAlias(mb.AccountID, id)
	if err := s.temp.Remove(id); err != nil {
		failCode(c, http.StatusInternalServerError, "INTERNAL_ERROR", "移除隨機信箱記錄失敗")
		return
	}
	resp := gin.H{"id": id, "email": mb.Email, "removed": true}
	if upstreamErr != nil {
		resp["upstream_warning"] = upstreamErr.Error()
	}
	ok(c, resp)
}

// now 是可注入的時鐘，測試用來控制到期時間。
func (s *Server) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// StartTempSweeper 啟動背景清理，直到 ctx 結束。
func (s *Server) StartTempSweeper(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(tempSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweepTempOnce()
			}
		}
	}()
}

// sweepTempOnce 刪除所有已到期的隨機信箱。
func (s *Server) sweepTempOnce() {
	due := s.temp.Due(s.now())
	for _, mb := range due {
		if err := s.be.DeleteAlias(mb.AccountID, mb.ID); err != nil {
			attempts := s.temp.RecordFailure(mb.ID, err.Error())
			if attempts >= maxTempDeleteAttempts {
				log.Printf("隨機信箱 %s 自動刪除連續失敗 %d 次，停止追蹤：%v", mb.Email, attempts, err)
				_ = s.temp.Remove(mb.ID)
				continue
			}
			log.Printf("隨機信箱 %s 自動刪除失敗（第 %d 次）：%v", mb.Email, attempts, err)
			continue
		}
		log.Printf("隨機信箱 %s 已到期自動刪除", mb.Email)
		_ = s.temp.Remove(mb.ID)
	}
}
