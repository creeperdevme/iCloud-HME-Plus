// Package server - 登入/工作階段/退出 handler 與認證中介軟體。
//
// auth.Manager 不依賴 Gin;此處只負責 Cookie/Header 與 HTTP 狀態映射。
package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/auth"
)

// authManager 是 auth.Manager 的別名,便於 handler 簽名。
type authManager = auth.Manager

// sessionCookieName 是管理員工作階段 Cookie 名稱。
const sessionCookieName = "hme_session"

// sessionIDFromCookie 從請求 Cookie 提取 session ID。
func sessionIDFromCookie(c *gin.Context) string {
	if cookie, err := c.Request.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	return ""
}

// requireSession 校驗工作階段,失敗返回 401/AUTH_REQUIRED。
func requireSession(mgr *authManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := sessionIDFromCookie(c)
		if sessionID == "" {
			failCode(c, http.StatusUnauthorized, "AUTH_REQUIRED", "請先登入")
			return
		}
		if _, ok := mgr.Validate(sessionID); !ok {
			failCode(c, http.StatusUnauthorized, "AUTH_REQUIRED", "工作階段已失效，請重新登入")
			return
		}
		c.Set("session_id", sessionID)
		c.Next()
	}
}

// setSessionCookie 設定工作階段 Cookie(固定屬性:Path=/、HttpOnly、SameSite=Strict)。
func setSessionCookie(c *gin.Context, sessionID string, expiresAt time.Time, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
	})
}

// clearSessionCookie 清除工作階段 Cookie。
func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// handleLogin 處理 POST /api/auth/login。
func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "參數錯誤")
		return
	}

	ip := c.ClientIP()
	allowed, retryAfter := s.limiter.Allow(ip)
	if !allowed {
		c.Header("Retry-After", formatRetryAfter(retryAfter))
		failCode(c, http.StatusTooManyRequests, "RATE_LIMITED", "登入嘗試過於頻繁，請稍後再試")
		return
	}

	sessionID, sess, valid := s.auth.Login(req.Password)
	if !valid {
		failCode(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "管理員密碼錯誤")
		return
	}
	s.limiter.Success(ip)
	setSessionCookie(c, sessionID, sess.ExpiresAt, s.cfg.SecureCookie)
	ok(c, gin.H{
		"csrf_token": sess.CSRFToken,
		"expires_at": sess.ExpiresAt.Format(time.RFC3339),
	})
}

// formatRetryAfter 把等待時間格式化為秒數(Retry-After 頭規範)。
func formatRetryAfter(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}

// handleSession 處理 GET /api/auth/session。
func (s *Server) handleSession(c *gin.Context) {
	sessionID := sessionIDFromCookie(c)
	if sessionID == "" {
		failCode(c, http.StatusUnauthorized, "AUTH_REQUIRED", "請先登入")
		return
	}
	sess, valid := s.auth.Validate(sessionID)
	if !valid {
		failCode(c, http.StatusUnauthorized, "AUTH_REQUIRED", "工作階段已失效，請重新登入")
		return
	}
	ok(c, gin.H{
		"csrf_token": sess.CSRFToken,
		"expires_at": sess.ExpiresAt.Format(time.RFC3339),
	})
}

// handleLogout 處理 POST /api/auth/logout(需工作階段 + CSRF)。
func (s *Server) handleLogout(c *gin.Context) {
	sessionID := sessionIDFromCookie(c)
	if sessionID != "" {
		s.auth.Logout(sessionID)
	}
	clearSessionCookie(c)
	ok(c, gin.H{"logged_out": true})
}
