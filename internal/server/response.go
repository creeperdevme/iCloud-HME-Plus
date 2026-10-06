// Package server - 統一回應格式與穩定錯誤碼。
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// apiResp 是統一 API 回應。
type apiResp struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// ok 返回統一成功回應。
func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, apiResp{Success: true, Data: data})
}

// failCode 返回統一失敗回應。
func failCode(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, apiResp{Success: false, Code: code, Message: message})
}

// backendFail 把 Backend 錯誤映射為統一失敗回應。
func backendFail(c *gin.Context, err error) {
	be := asBackendError(err)
	failCode(c, be.Status, be.Code, be.Message)
}
