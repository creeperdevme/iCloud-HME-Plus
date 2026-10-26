package server

import (
	"github.com/gin-gonic/gin"
	"icloud-hme/internal/webhook"
	"net/http"
	"strings"
)

func (s *Server) listAPIKeys(c *gin.Context) { ok(c, s.keys.List()) }
func (s *Server) createAPIKey(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failCode(c, 400, "VALIDATION_ERROR", "參數錯誤")
		return
	}
	pub, token, err := s.keys.Create(in.Name)
	if err != nil {
		failCode(c, 500, "INTERNAL_ERROR", "建立 API Key 失敗")
		return
	}
	createdOK(c, gin.H{"key": token, "id": pub.ID, "name": pub.Name, "created_at": pub.CreatedAt})
}
func (s *Server) revokeAPIKey(c *gin.Context) {
	if !s.keys.Revoke(c.Param("id")) {
		failCode(c, 404, "NOT_FOUND", "API Key 不存在或已撤銷")
		return
	}
	ok(c, gin.H{"revoked": true})
}
func (s *Server) webhookExamples(c *gin.Context) {
	ok(c, []webhook.Input{
		{Name: "Telegram", Type: "telegram", URL: "https://api.telegram.org/bot<TOKEN>/sendMessage", Events: []string{"email_received"}},
		{Name: "Discord", Type: "discord", URL: "https://discord.com/api/webhooks/<ID>/<TOKEN>", Events: []string{"email_received"}},
		{Name: "Bark", Type: "bark", URL: "https://api.day.app/<KEY>", Events: []string{"email_received"}},
	})
}
func (s *Server) listWebhooks(c *gin.Context) { ok(c, s.webhooks.List()) }
func (s *Server) createWebhook(c *gin.Context) {
	var in webhook.Input
	if c.ShouldBindJSON(&in) != nil {
		failCode(c, 400, "VALIDATION_ERROR", "參數錯誤")
		return
	}
	w, err := s.webhooks.Create(in)
	if err != nil {
		failCode(c, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	createdOK(c, webhook.PublicView(w))
}
func (s *Server) updateWebhook(c *gin.Context) {
	var in webhook.Input
	if c.ShouldBindJSON(&in) != nil {
		failCode(c, 400, "VALIDATION_ERROR", "參數錯誤")
		return
	}
	w, err := s.webhooks.Update(c.Param("id"), in)
	if err != nil {
		failCode(c, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	ok(c, webhook.PublicView(w))
}
func (s *Server) deleteWebhook(c *gin.Context) {
	if !s.webhooks.Delete(c.Param("id")) {
		failCode(c, 404, "NOT_FOUND", "Webhook 不存在")
		return
	}
	ok(c, gin.H{"deleted": true})
}
func (s *Server) testWebhook(c *gin.Context) {
	if err := s.webhooks.Dispatch("test", gin.H{"message": "iCloud HME webhook test"}); err != nil {
		failCode(c, 502, "UPSTREAM_FAILURE", "Webhook 發送失敗")
		return
	}
	ok(c, gin.H{"sent": true})
}

func (s *Server) handleMCPInfo(c *gin.Context) {
	ok(c, gin.H{"name": "iCloud HME MCP Server", "transport": "HTTP", "endpoint": "/api/mcp", "authentication": "Bearer API key", "tools": []string{"list_accounts", "create_alias", "list_inbox", "get_verification_code"}})
}
func (s *Server) handleMCP(c *gin.Context) {
	authz := c.GetHeader("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		failCode(c, http.StatusUnauthorized, "AUTH_REQUIRED", "需要 Bearer API Key")
		return
	}
	if _, valid := s.keys.Authenticate(strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))); !valid {
		failCode(c, http.StatusUnauthorized, "INVALID_API_KEY", "API Key 無效")
		return
	}
	var req struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if c.ShouldBindJSON(&req) != nil {
		failCode(c, 400, "VALIDATION_ERROR", "MCP 請求格式錯誤")
		return
	}
	switch req.Method {
	case "tools/list":
		ok(c, gin.H{"tools": []any{gin.H{"name": "list_accounts"}, gin.H{"name": "create_alias"}, gin.H{"name": "list_inbox"}, gin.H{"name": "get_verification_code"}}})
	default:
		failCode(c, 404, "MCP_METHOD_NOT_FOUND", "MCP 方法不存在")
	}
}
