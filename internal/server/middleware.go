// Package server - 安全中介軟體:請求上限、安全回應頭、CSRF 校驗。
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// maxBodyBytes 是 JSON 請求體上限。
const maxBodyBytes = 1 << 20 // 1 MiB

// securityHeaders 是全局安全回應頭。
var securityHeaders = map[string]string{
	"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
	"X-Content-Type-Options":  "nosniff",
	"Referrer-Policy":         "no-referrer",
	"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
}

// securityHeadersMiddleware 設定全局安全回應頭。
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		for k, v := range securityHeaders {
			c.Header(k, v)
		}
		c.Next()
	}
}

// apiCacheControlMiddleware 給 API 回應設定 no-store。
func apiCacheControlMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// csrfCheck 校驗狀態變更請求的 CSRF token。
func csrfCheck(mgr *authManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := sessionIDFromCookie(c)
		if sessionID == "" {
			failCode(c, http.StatusForbidden, "CSRF_INVALID", "缺少工作階段")
			return
		}
		token := c.GetHeader("X-CSRF-Token")
		if token == "" || !mgr.ValidateCSRF(sessionID, token) {
			failCode(c, http.StatusForbidden, "CSRF_INVALID", "CSRF 校驗失敗")
			return
		}
		c.Next()
	}
}
