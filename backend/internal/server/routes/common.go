package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}

// RegisterPluginPublicRoutes 注册公共的插件清单路由
// 向前端暴露当前启用的插件清单（菜单、表单 schema、UI bundle），
// 不需要认证。仅返回 state=enabled 的插件，secret 字段的默认值会被屏蔽。
func RegisterPluginPublicRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	if h == nil || h.PluginList == nil {
		return
	}
	v1.GET("/plugins", h.PluginList.ListEnabledPlugins)
}
