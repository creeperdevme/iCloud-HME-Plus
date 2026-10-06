// Package server 提供 HTTP API,基於 Gin。
//
// 兩個核心介面:
//
//	POST /api/create  — 在指定帳號下建立一個 Hide My Email 別名
//	GET  /api/inbox   — 讀取指定帳號(或指定別名)收到的郵件
//
// 輔助介面(用於多帳號管理):帳號增刪查、別名列表、設定 App 密碼。
//
// 安全模型:除 /api/auth/login 與 /api/auth/session 外,所有 /api 路由都需要
// 管理員工作階段;非 GET/HEAD/OPTIONS 請求還需校驗 CSRF。
package server

import (
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/account"
	"icloud-hme/internal/auth"
	"icloud-hme/internal/webui"
)

// Config 是 Server 的啟動配置。
type Config struct {
	Debug         bool
	AdminPassword string
	SessionTTL    time.Duration
	SecureCookie  bool
}

// Server 封裝 Gin 引擎、帳號後端與認證。
type Server struct {
	be      Backend
	auth    *auth.Manager
	limiter *auth.Limiter
	cfg     Config
	r       *gin.Engine
}

// New 建立 Server。mgr 為帳號管理器,cfg 為安全配置。
func New(mgr *account.Manager, cfg Config) (*Server, error) {
	if _, err := auth.NewManager(auth.Options{
		Password: cfg.AdminPassword,
		TTL:      cfg.SessionTTL,
	}); err != nil {
		return nil, err
	}
	return newWithBackend(&managerBackend{mgr: mgr}, cfg), nil
}

// newWithBackend 建立 Server 並注入 Backend(測試使用記憶體 fake)。
func newWithBackend(be Backend, cfg Config) *Server {
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{
		be:      be,
		limiter: auth.NewLimiter(nil, 15*time.Minute, 5, 10000),
		cfg:     cfg,
	}
	s.auth, _ = auth.NewManager(auth.Options{
		Password: cfg.AdminPassword,
		TTL:      cfg.SessionTTL,
	})
	s.r = gin.New()
	s.r.Use(gin.Logger(), gin.Recovery(), securityHeadersMiddleware())
	// 不信任任意代理頭,登入限流使用真實連線 IP
	_ = s.r.SetTrustedProxies(nil)
	s.register()
	return s
}

// Run 啟動 HTTP 服務。
func (s *Server) Run(addr string) error {
	return s.r.Run(addr)
}

// Handler 返回底層 gin 引擎(便於測試)。
func (s *Server) Handler() http.Handler { return s.r }

func (s *Server) register() {
	api := s.r.Group("/api")
	api.Use(apiCacheControlMiddleware())
	{
		// ===== 認證(公開) =====
		api.POST("/auth/login", s.handleLogin)
		api.GET("/auth/session", s.handleSession)

		// ===== 受保護路由:統一 requireSession =====
		authed := api.Group("")
		authed.Use(requireSession(s.auth))
		{
			authed.POST("/auth/logout", csrfCheck(s.auth), s.handleLogout)

			// ===== 帳號管理 =====
			authed.GET("/accounts", s.listAccountsHandler)
			authed.POST("/accounts", csrfCheck(s.auth), s.addAccountHandler)
			authed.PATCH("/accounts/:id", csrfCheck(s.auth), s.updateAccountHandler)
			authed.PUT("/accounts/:id/proxy", csrfCheck(s.auth), s.updateProxyHandler)
			authed.PUT("/accounts/:id/cookies", csrfCheck(s.auth), s.updateCookiesHandler)
			authed.POST("/accounts/:id/password", csrfCheck(s.auth), s.setAppPasswordHandler)
			authed.PUT("/accounts/:id/mailbox", csrfCheck(s.auth), s.setMailboxHandler)
			authed.POST("/accounts/:id/login", csrfCheck(s.auth), s.loginAccountHandler)
			authed.DELETE("/accounts/:id", csrfCheck(s.auth), s.removeAccountHandler)

			// ===== 核心介面 1: 建立信箱 =====
			authed.POST("/create", csrfCheck(s.auth), s.createAliasHandler)

			// ===== 核心介面 2: 讀取郵件 =====
			authed.GET("/inbox", s.listInboxHandler)
			authed.GET("/inbox/:message_id", s.getMessageHandler)
			authed.DELETE("/inbox/:message_id", csrfCheck(s.auth), s.deleteMessageHandler)

			// ===== 別名管理 =====
			authed.GET("/aliases", s.listAliasesHandler)
			authed.POST("/aliases/:id/deactivate", csrfCheck(s.auth), s.deactivateAliasHandler)
			authed.POST("/aliases/:id/reactivate", csrfCheck(s.auth), s.reactivateAliasHandler)
			authed.DELETE("/aliases/:id", csrfCheck(s.auth), s.deleteAliasHandler)

			// ===== 系統 =====
			authed.POST("/reload", csrfCheck(s.auth), s.reloadConfigHandler)
		}
	}
	// API 404 返回 JSON,絕不讓 NoRoute 把拼錯的 API 路徑變成 HTML
	s.r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			failCode(c, http.StatusNotFound, "VALIDATION_ERROR", "介面不存在")
			return
		}
		// 其餘路徑交給 webui(SPA fallback)
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.String(http.StatusMethodNotAllowed, "不允許的方法")
			return
		}
		webui.Handler(webuiFS).ServeHTTP(c.Writer, c.Request)
	})
}

// webuiFS 是內嵌前端資源(可被測試替換)。
var webuiFS = func() fs.FS {
	f, err := webui.Embedded()
	if err != nil {
		return nil
	}
	return f
}()

// ====================================================================
// 核心介面 1: 建立信箱
//   POST /api/create
//   body: {"account_id": "acc_xxx", "label": "可選標簽"}
//   返回: 新建立的 HME 信箱位址
// ====================================================================

type createAliasReq struct {
	AccountID string `json:"account_id"`
	Label     string `json:"label"`
}

func (s *Server) createAliasHandler(c *gin.Context) {
	var req createAliasReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AccountID == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：account_id 必填")
		return
	}
	if len([]rune(req.Label)) > 200 {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：label 最長 200 字元")
		return
	}

	result, err := s.be.CreateAlias(req.AccountID, req.Label)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{
		"email":      result.Email,
		"label":      result.Label,
		"created_at": result.CreatedAt,
		"account_id": req.AccountID,
	})
}

// ====================================================================
// 核心介面 2: 讀取郵件
//   GET /api/inbox?account_id=acc_xxx[&alias=xxx@icloud.com][&limit=20][&days=7]
//
//   - 不傳 alias: 返回該帳號收件匣最近郵件
//   - 傳 alias:   只返回發給該 HME 別名的郵件
//
//   認證優先級: IMAP (App Password) 優先 > Web API (Cookie) 回退
// ====================================================================

func (s *Server) listInboxHandler(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "缺少參數：account_id")
		return
	}
	alias := strings.TrimSpace(c.Query("alias"))
	limit, err := parseInboxInt(c.DefaultQuery("limit", "20"), 1, 100)
	if err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：limit 需為 1-100 的整數")
		return
	}
	days, err := parseInboxInt(c.DefaultQuery("days", "7"), 1, 90)
	if err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：days 需為 1-90 的整數")
		return
	}

	result, err := s.be.ListInbox(InboxQuery{
		AccountID: accountID,
		Alias:     alias,
		Limit:     limit,
		Days:      days,
	})
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, result)
}

func (s *Server) getMessageHandler(c *gin.Context) {
	accountID := c.Query("account_id")
	uid, err := strconv.ParseUint(c.Param("message_id"), 10, 32)
	if accountID == "" || err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "account_id 或郵件 ID 無效")
		return
	}
	message, err := s.be.GetMessage(accountID, uint32(uid))
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, message)
}

func (s *Server) deleteMessageHandler(c *gin.Context) {
	accountID := c.Query("account_id")
	uid, err := strconv.ParseUint(c.Param("message_id"), 10, 32)
	if accountID == "" || err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "account_id 或郵件 ID 無效")
		return
	}
	if err := s.be.DeleteMessage(accountID, uint32(uid)); err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{"id": c.Param("message_id")})
}

// parseInboxInt 解析整數參數,非法或越界返回錯誤(不再靜默變成 0)。
func parseInboxInt(raw string, min, max int) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, errors.New("invalid integer")
	}
	if v < min || v > max {
		return 0, errors.New("out of range")
	}
	return v, nil
}

// ====================================================================
// 輔助介面:別名
// ====================================================================

func (s *Server) listAliasesHandler(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "缺少參數：account_id")
		return
	}
	aliases, err := s.be.ListAliases(accountID)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{
		"account_id": accountID,
		"count":      len(aliases),
		"aliases":    aliases,
	})
}

type aliasActionReq struct {
	AccountID string `json:"account_id"`
}

// validateAliasAction 校驗別名操作的匿名 ID 與請求體。
func validateAliasAction(c *gin.Context) (accountID, anonymousID string, valid bool) {
	anonymousID = c.Param("id")
	var req aliasActionReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AccountID == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：account_id 必填")
		return "", "", false
	}
	if anonymousID == "" || len(anonymousID) > 256 {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤：anonymous id 無效")
		return "", "", false
	}
	return req.AccountID, anonymousID, true
}

func (s *Server) deactivateAliasHandler(c *gin.Context) {
	accountID, anonymousID, valid := validateAliasAction(c)
	if !valid {
		return
	}
	success, err := s.be.SetAliasActive(accountID, anonymousID, false)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{"anonymous_id": anonymousID, "success": success})
}

func (s *Server) reactivateAliasHandler(c *gin.Context) {
	accountID, anonymousID, valid := validateAliasAction(c)
	if !valid {
		return
	}
	success, err := s.be.SetAliasActive(accountID, anonymousID, true)
	if err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{"anonymous_id": anonymousID, "success": success})
}

func (s *Server) deleteAliasHandler(c *gin.Context) {
	accountID, anonymousID, valid := validateAliasAction(c)
	if !valid {
		return
	}
	if err := s.be.DeleteAlias(accountID, anonymousID); err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{"anonymous_id": anonymousID})
}

// ====================================================================
// 系統
// ====================================================================

func (s *Server) reloadConfigHandler(c *gin.Context) {
	if err := s.be.Reload(); err != nil {
		backendFail(c, err)
		return
	}
	ok(c, gin.H{"message": "設定已重新載入"})
}
