// Package server - 帳號管理 handler。
//
// 只做綁定、校驗、呼叫 Backend 和回應映射;帳號介面統一返回無秘密的
// account.Summary。任何回應不得包含 cookies、app_password、proxy。
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/account"
)

// listAccountsHandler 處理 GET /api/accounts。
func (s *Server) listAccountsHandler(c *gin.Context) {
	ok(c, s.be.ListAccounts())
}

// addAccountReq 是 POST /api/accounts 請求體。
//
// AppPassword 選填:與 Cookie 一樣可以在新增時就一起帶上,不必先建好帳號
// 再另外設定。驗證失敗不會讓新增失敗,只會以 Warning 回報。
type addAccountReq struct {
	Name        string `json:"name"`
	ICloudEmail string `json:"icloud_email"`
	Cookies     string `json:"cookies"`
	Host        string `json:"host"`
	Proxy       string `json:"proxy"`
	AppPassword string `json:"app_password"`
}

// appPasswordWarning 是「帳號已建立但 App 專用密碼沒存進去」的提示文字。
const appPasswordWarning = "帳號已建立，但 App 專用密碼未通過 IMAP 驗證，因此尚未設定；請確認密碼後用「App 密碼」重新設定。"

// addAccountHandler 處理 POST /api/accounts。
func (s *Server) addAccountHandler(c *gin.Context) {
	var req addAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤")
		return
	}
	sum, err := s.be.AddAccount(account.AddAccountInput{
		Name:        req.Name,
		ICloudEmail: req.ICloudEmail,
		CookieInput: req.Cookies,
		Host:        req.Host,
		Proxy:       req.Proxy,
		AppPassword: req.AppPassword,
	})
	if err != nil {
		backendFail(c, err)
		return
	}
	// 有送密碼但摘要顯示沒存到,代表 IMAP 驗證沒過:帳號留著,另外提醒。
	if req.AppPassword != "" && !sum.HasAppPassword {
		createdWithWarning(c, sum, appPasswordWarning)
		return
	}
	createdOK(c, sum)
}

// updateAccountReq 是 PATCH /api/accounts/:id 請求體。
type updateAccountReq struct {
	Name        *string `json:"name"`
	ICloudEmail *string `json:"icloud_email"`
	Host        *string `json:"host"`
}

// updateAccountHandler 處理 PATCH /api/accounts/:id。
func (s *Server) updateAccountHandler(c *gin.Context) {
	id := c.Param("id")
	var req updateAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤")
		return
	}
	sum, err := s.be.UpdateAccount(id, account.UpdateAccountInput{
		Name:        req.Name,
		ICloudEmail: req.ICloudEmail,
		Host:        req.Host,
	})
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, sum)
}

// proxyReq 是 PUT /api/accounts/:id/proxy 請求體。
type proxyReq struct {
	Proxy string `json:"proxy"`
}

// updateProxyHandler 處理 PUT /api/accounts/:id/proxy。
func (s *Server) updateProxyHandler(c *gin.Context) {
	id := c.Param("id")
	var req proxyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤")
		return
	}
	sum, err := s.be.UpdateProxy(id, req.Proxy)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, sum)
}

// updateCookiesHandler 處理 PUT /api/accounts/:id/cookies。
//
// cookies 同時兼容字串與物件;兩種輸入最終都交給 account.ParseCookieInput。
func (s *Server) updateCookiesHandler(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Cookies json.RawMessage `json:"cookies"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Cookies) == 0 {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：cookies 必填")
		return
	}

	var raw string
	var asText string
	if json.Unmarshal(req.Cookies, &asText) == nil {
		raw = asText
	} else {
		var asMap map[string]string
		if err := json.Unmarshal(req.Cookies, &asMap); err != nil {
			failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：cookies 格式無效")
			return
		}
		raw = cookieInputToJSON(asMap)
	}

	sum, err := s.be.UpdateCookies(id, raw)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, sum)
}

// setAppPasswordReq 是 POST /api/accounts/:id/password 請求體。
//
// ICloudEmail 選填:從既有帳號開啟時通常已經有信箱了,留空就沿用帳號上
// 已儲存的位址,使用者不必再打一次完整信箱。
type setAppPasswordReq struct {
	ICloudEmail string `json:"icloud_email"`
	AppPassword string `json:"app_password"`
}

// setAppPasswordHandler 處理 POST /api/accounts/:id/password。
func (s *Server) setAppPasswordHandler(c *gin.Context) {
	id := c.Param("id")
	var req setAppPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AppPassword == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：app_password 必填")
		return
	}

	email := strings.TrimSpace(req.ICloudEmail)
	if email == "" {
		// 未帶信箱時沿用帳號上已儲存的,維持「既有帳號不必重填」的行為。
		sum, found := s.be.GetAccount(id)
		if !found {
			failCode(c, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "帳號不存在")
			return
		}
		email = strings.TrimSpace(sum.ICloudEmail)
		if email == "" {
			failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "此帳號尚未設定 iCloud 信箱，請先填寫完整信箱")
			return
		}
	}

	sum, err := s.be.SetAppPassword(id, email, req.AppPassword)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, sum)
}

type setMailboxReq struct {
	Provider          string `json:"provider"`
	Email             string `json:"email"`
	IMAPHost          string `json:"imap_host"`
	IMAPPort          int    `json:"imap_port"`
	AuthorizationCode string `json:"authorization_code"`
}

func (s *Server) setMailboxHandler(c *gin.Context) {
	var req setMailboxReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" || req.IMAPHost == "" || req.IMAPPort < 1 || req.AuthorizationCode == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：收件信箱、IMAP 伺服器、連接埠與授權碼必填")
		return
	}
	sum, err := s.be.SetMailbox(c.Param("id"), account.MailboxConfig{
		Provider: req.Provider, Email: req.Email, IMAPHost: req.IMAPHost, IMAPPort: req.IMAPPort, Password: req.AuthorizationCode,
	})
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, sum)
}

// loginAccountReq 是 POST /api/accounts/:id/login 請求體。
type loginAccountReq struct {
	Password string `json:"password"`
	OTPCode  string `json:"otp_code"`
}

// loginAccountHandler 處理 POST /api/accounts/:id/login。
//
// 成功只返回 Summary,絕不返回 Cookies。
func (s *Server) loginAccountHandler(c *gin.Context) {
	id := c.Param("id")
	var req loginAccountReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：password 必填")
		return
	}
	sum, err := s.be.LoginAccount(id, req.Password, req.OTPCode)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, sum)
}

// removeAccountHandler 處理 DELETE /api/accounts/:id。
func (s *Server) removeAccountHandler(c *gin.Context) {
	id := c.Param("id")
	if !s.be.RemoveAccount(id) {
		failCode(c, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "帳號不存在")
		return
	}
	ok(c, gin.H{"id": id})
}
