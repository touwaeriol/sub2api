package routes

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/gin-gonic/gin"
)

// RegisterDocsRoutes 注册文档相关路由
func RegisterDocsRoutes(v1 *gin.RouterGroup) {
	docs := v1.Group("/docs")
	{
		// GET /api/v1/docs/config — 返回文档导航配置
		docs.GET("/config", func(c *gin.Context) {
			data, err := os.ReadFile(docsFilePath("config.json"))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"groups": []any{}})
				return
			}
			c.Data(http.StatusOK, "application/json; charset=utf-8", data)
		})

		// GET /api/v1/docs/:slug — 返回单篇文档的 Markdown 内容
		docs.GET("/:slug", func(c *gin.Context) {
			slug := c.Param("slug")

			// 安全校验：禁止路径穿越
			if strings.Contains(slug, "..") || strings.Contains(slug, "/") || strings.Contains(slug, "\\") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
				return
			}

			filename := slug + ".md"
			data, err := os.ReadFile(docsFilePath(filename))
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
				return
			}
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", data)
		})
	}
}

// docsFilePath 返回文档文件的完整路径
func docsFilePath(name string) string {
	return filepath.Join(setup.GetDataDir(), "docs", name)
}
