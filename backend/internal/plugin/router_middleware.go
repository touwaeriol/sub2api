package plugin

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
)

// PluginRouter 是核心 HTTP 入口的包装层。
// 请求进来后先尝试匹配插件路由表;若命中则按鉴权要求执行中间件,然后反向代理到插件进程;
// 若未命中则交给原 Gin 引擎处理。
type PluginRouter struct {
	routeTable  atomic.Pointer[RouteTable]
	coreHandler http.Handler

	// authEngines 按鉴权类型缓存预构建的 gin.Engine,避免每次请求分配。
	authEngines map[string]*gin.Engine

	// proxies 缓存反向代理实例,避免每次请求重新构造。
	proxies sync.Map // map[string]*httputil.ReverseProxy
}

// authCapture 是请求级别的捕获容器,通过 request context 传递给 auth engine 的 catch-all handler。
// gin.Engine.ServeHTTP 同步执行中间件链,因此 capture 写入与读取在同一 goroutine,无竞态。
type authCapture struct {
	ginCtx *gin.Context
}

type authCaptureKeyType struct{}

var authCaptureKey authCaptureKeyType

// NewPluginRouter 构造插件路由器,coreHandler 通常是 *gin.Engine。
// 三个鉴权中间件在请求被代理到插件前执行,与正式路由享有完全一致的鉴权语义。
func NewPluginRouter(coreHandler http.Handler, jwtAuth, adminAuth, apiKeyAuth gin.HandlerFunc) *PluginRouter {
	r := &PluginRouter{
		coreHandler: coreHandler,
		authEngines: buildAuthEngines(jwtAuth, adminAuth, apiKeyAuth),
	}
	r.routeTable.Store(NewRouteTable())
	return r
}

// buildAuthEngines 为每种鉴权类型预构建一个 gin.Engine,构造一次后在所有请求中复用。
// gin.Engine.ServeHTTP 是并发安全的(内部用 sync.Pool 管理 Context)。
func buildAuthEngines(jwtAuth, adminAuth, apiKeyAuth gin.HandlerFunc) map[string]*gin.Engine {
	engines := make(map[string]*gin.Engine, 3)
	pairs := []struct {
		authType string
		handler  gin.HandlerFunc
	}{
		{AuthTypeAdmin, adminAuth},
		{AuthTypeUser, jwtAuth},
		{AuthTypeAPIKey, apiKeyAuth},
	}
	for _, p := range pairs {
		if p.handler == nil {
			continue
		}
		engines[p.authType] = newAuthEngine(p.handler)
	}
	return engines
}

// newAuthEngine 构造带单个鉴权中间件的 gin.Engine。
// 鉴权通过后,catch-all handler 把 gin.Context 写入 request context 中的 authCapture。
func newAuthEngine(mw gin.HandlerFunc) *gin.Engine {
	engine := gin.New()
	engine.Use(mw)
	engine.Any("/*any", captureGinContext)
	return engine
}

// captureGinContext 是 auth engine 的 catch-all handler,鉴权通过时将 gin.Context
// 写入 request context 中的 authCapture。若鉴权 abort,此 handler 不会被执行。
func captureGinContext(c *gin.Context) {
	if cap, ok := c.Request.Context().Value(authCaptureKey).(*authCapture); ok {
		cap.ginCtx = c
	}
}

// SwapRouteTable 原子地替换路由表,供 manager 在插件加载/卸载时调用。
// 同时清理不再被任何路由引用的反向代理缓存条目。
func (r *PluginRouter) SwapRouteTable(table *RouteTable) {
	if table == nil {
		table = NewRouteTable()
	}
	r.routeTable.Store(table)
	r.pruneStaleProxies(table)
}

// RemovePlugin 清理指定插件在 proxies sync.Map 中的缓存条目。
// manager 在卸载 / 重启插件时,应在 SwapRouteTable 后调用此方法,
// 确保旧端口对应的 ReverseProxy 实例被释放。
func (r *PluginRouter) RemovePlugin(proxyURL string) {
	if proxyURL != "" {
		r.proxies.Delete(proxyURL)
	}
}

// pruneStaleProxies 遍历 proxies 缓存,删除不在新路由表中出现的条目。
func (r *PluginRouter) pruneStaleProxies(table *RouteTable) {
	active := table.collectProxyURLs()
	r.proxies.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok {
			if _, inUse := active[k]; !inUse {
				r.proxies.Delete(k)
			}
		}
		return true
	})
}

// CurrentTable 返回当前路由表快照,主要供测试与状态接口使用。
func (r *PluginRouter) CurrentTable() *RouteTable {
	return r.routeTable.Load()
}

// ServeHTTP 实现 http.Handler 接口。
func (r *PluginRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	table := r.routeTable.Load()
	entry, ok := table.Match(req.Method, req.URL.Path)
	if !ok {
		r.coreHandler.ServeHTTP(w, req)
		return
	}

	authCtx, ok := r.runAuthMiddleware(w, req, entry.AuthType)
	if !ok {
		return
	}

	r.proxyTo(w, req, authCtx, entry)
}

// runAuthMiddleware 通过预构建的 gin.Engine 执行匹配 authType 的鉴权中间件。
// 返回的 *gin.Context 在通过时携带鉴权信息(无鉴权时返回 nil ctx + ok=true)。
// ok=false 表示鉴权失败,响应已经被中间件写出,调用方应直接返回。
func (r *PluginRouter) runAuthMiddleware(w http.ResponseWriter, req *http.Request, authType string) (*gin.Context, bool) {
	if authType == "" || authType == AuthTypeNone {
		return nil, true
	}

	engine, exists := r.authEngines[authType]
	if !exists {
		http.Error(w, "auth middleware not available", http.StatusForbidden)
		return nil, false
	}

	return runPrebuiltAuthEngine(engine, w, req)
}

// runPrebuiltAuthEngine 在预构建 engine 上执行鉴权,通过 authCapture 提取 gin.Context。
// gin.Engine.ServeHTTP 在同一 goroutine 同步执行整个中间件链,因此 capture
// 的写入(captureGinContext)和读取(本函数返回后)之间不存在竞态。
func runPrebuiltAuthEngine(engine *gin.Engine, w http.ResponseWriter, req *http.Request) (*gin.Context, bool) {
	cap := &authCapture{}
	ctx := context.WithValue(req.Context(), authCaptureKey, cap)
	engine.ServeHTTP(w, req.WithContext(ctx))
	if cap.ginCtx == nil {
		return nil, false
	}
	return cap.ginCtx, true
}

// proxyTo 把请求反向代理到插件进程。
func (r *PluginRouter) proxyTo(w http.ResponseWriter, req *http.Request, authCtx *gin.Context, entry *RouteEntry) {
	target, err := url.Parse(entry.ProxyURL)
	if err != nil {
		http.Error(w, "invalid plugin upstream", http.StatusBadGateway)
		return
	}

	proxy := r.getOrCreateProxy(entry.ProxyURL, target)
	injectPluginHeaders(req, authCtx, entry.PluginName)
	stripHopByHopHeaders(req.Header)
	proxy.ServeHTTP(w, req)
}

// injectPluginHeaders 注入用户上下文头,使插件不必再次解析 JWT/APIKey。
func injectPluginHeaders(req *http.Request, authCtx *gin.Context, pluginName string) {
	if authCtx != nil {
		if subject, ok := middleware.GetAuthSubjectFromContext(authCtx); ok && subject.UserID > 0 {
			req.Header.Set("X-Plugin-User-ID", strconv.FormatInt(subject.UserID, 10))
		}
		if role, ok := middleware.GetUserRoleFromContext(authCtx); ok && role != "" {
			req.Header.Set("X-Plugin-User-Role", role)
		}
	}
	req.Header.Set("X-Plugin-Name", pluginName)
}

func (r *PluginRouter) getOrCreateProxy(key string, target *url.URL) *httputil.ReverseProxy {
	if v, ok := r.proxies.Load(key); ok {
		return v.(*httputil.ReverseProxy)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.Host = target.Host
	}
	actual, _ := r.proxies.LoadOrStore(key, proxy)
	return actual.(*httputil.ReverseProxy)
}

// hop-by-hop 头列表来自 RFC 7230 §6.1。
var hopByHopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func stripHopByHopHeaders(h http.Header) {
	if connection := h.Get("Connection"); connection != "" {
		for _, f := range strings.Split(connection, ",") {
			if name := strings.TrimSpace(f); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}
