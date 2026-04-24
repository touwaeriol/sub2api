package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	// Built-in plugins: blank imports trigger their init() which calls
	// plugin.Register on the SDK global registry.
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	pluginmount "github.com/Wei-Shaw/sub2api/internal/plugin/mount"
	_ "github.com/Wei-Shaw/sub2api/internal/plugins/demo"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/Wei-Shaw/sub2api/internal/web"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags)
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source" // "source" for manual builds, "release" for CI builds (set by ldflags)
)

func init() {
	// 如果 Version 已通过 ldflags 注入（例如 -X main.Version=...），则不要覆盖。
	if strings.TrimSpace(Version) != "" {
		return
	}

	// 默认从 embedded VERSION 文件读取版本号（编译期打包进二进制）。
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

// initLogger configures the default slog handler based on gin.Mode().
// In non-release mode, Debug level logs are enabled.
func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	// Parse command line flags
	setupMode := flag.Bool("setup", false, "Run setup wizard in CLI mode")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	// CLI setup mode
	if *setupMode {
		if err := setup.RunCLI(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	// Check if setup is needed
	if setup.NeedsSetup() {
		// Check if auto-setup is enabled (for Docker deployment)
		if setup.AutoSetupEnabled() {
			log.Println("Auto setup mode enabled...")
			if err := setup.AutoSetupFromEnv(); err != nil {
				log.Fatalf("Auto setup failed: %v", err)
			}
			// Continue to main server after auto-setup
		} else {
			log.Println("First run detected, starting setup wizard...")
			runSetupServer()
			return
		}
	}

	// Normal server mode
	runMainServer()
}

func runSetupServer() {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(config.CORSConfig{}))
	r.Use(middleware.SecurityHeaders(config.CSPConfig{Enabled: true, Policy: config.DefaultCSPPolicy}, nil))

	// Register setup routes
	setup.RegisterRoutes(r)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
	}

	// Get server address from config.yaml or environment variables (SERVER_HOST, SERVER_PORT)
	// This allows users to run setup on a different address if needed
	addr := config.GetServerAddress()
	log.Printf("Setup wizard available at http://%s", addr)
	log.Println("Complete the setup wizard to configure Sub2API")

	server := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(r, &http2.Server{}),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start setup server: %v", err)
	}
}

func runMainServer() {
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	if cfg.RunMode == config.RunModeSimple {
		log.Println("⚠️  WARNING: Running in SIMPLE mode - billing and quota checks are DISABLED")
	}

	buildInfo := handler.BuildInfo{
		Version:   Version,
		BuildType: BuildType,
	}

	app, err := initializeApplication(buildInfo)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.Cleanup()

	// 插件子系统启动：核心 schema 已在 ProvideEventBus 注册；这里
	// 1) 启动事件总线后台分发；2) 重放数据库里 enabled 状态的插件；
	// 3) 把插件声明的 Routes / Subscribes 挂到主路由和事件总线。
	// 任何步骤失败都只记录日志、不阻断主服务启动。
	bootstrapPlugins(app)

	// 启动服务器
	go func() {
		if err := app.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started on %s", app.Server.Addr)

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 在关闭 HTTP / 基础设施之前，先优雅停掉 enabled 插件，确保
	// 插件 Shutdown 钩子能调到 Redis / DB。
	shutdownPlugins(ctx, app)

	if err := app.Server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

// bootstrapPlugins runs the plugin lifecycle replay + resource mounting
// after the main application has been wired but before HTTP starts.
// All failures are non-fatal — the host must keep serving traffic even if
// every plugin is broken.
func bootstrapPlugins(app *Application) {
	bootstrapCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.EventBus.Start(bootstrapCtx); err != nil {
		slog.Error("plugin event bus start failed", "error", err)
	}
	if err := app.PluginLoader.BootstrapAll(bootstrapCtx); err != nil {
		slog.Error("plugin bootstrap failed", "error", err)
	}
	authMW := pluginmount.AuthMiddlewares{
		User:  gin.HandlerFunc(app.JWTAuth),
		Admin: gin.HandlerFunc(app.AdminAuth),
	}
	if err := pluginmount.MountRoutes(bootstrapCtx, app.Router, app.PluginLoader, authMW, slog.Default()); err != nil {
		slog.Error("plugin route mount failed", "error", err)
	}
	if err := pluginmount.MountSubscriptions(bootstrapCtx, app.EventBus, app.PluginLoader, slog.Default()); err != nil {
		slog.Error("plugin subscription mount failed", "error", err)
	}
}

// shutdownPlugins disables every running plugin and stops the event bus.
// Called from the shutdown path so plugin teardown happens while DB /
// Redis are still alive.
func shutdownPlugins(ctx context.Context, app *Application) {
	app.PluginLoader.DisableAll(ctx)
	if err := app.EventBus.Stop(ctx); err != nil {
		slog.Error("plugin event bus stop failed", "error", err)
	}
}
