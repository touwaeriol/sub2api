package routes

import (
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterMonitorRoutes 注册公开监控路由（无需认证，带速率限制）
func RegisterMonitorRoutes(v1 *gin.RouterGroup, h *handler.Handlers, redisClient *redis.Client) {
	log.Printf("[Monitor] RegisterMonitorRoutes called, h.Monitor=%v", h.Monitor)
	if h.Monitor == nil {
		log.Printf("[Monitor] WARNING: h.Monitor is nil, skipping route registration")
		return
	}
	log.Printf("[Monitor] Registering GET /api/v1/monitor/:platform/:group_name")

	rateLimiter := middleware.NewRateLimiter(redisClient)

	monitor := v1.Group("/monitor")
	monitor.Use(rateLimiter.LimitWithOptions("monitor", 30, time.Minute, middleware.RateLimitOptions{
		FailureMode: middleware.RateLimitFailOpen,
	}))
	{
		monitor.GET("/group", h.Monitor.GetGroupAccountMonitor)
	}
}
