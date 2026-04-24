// Package mount wires enabled plugins' declared resources (HTTP routes and
// event subscriptions) into the host's gin router and event bus after
// Loader.BootstrapAll has transitioned their rows to "enabled".
//
// Kept deliberately small: the Loader owns lifecycle state, the mount
// package only translates Meta.Routes / Meta.Subscribes into concrete
// registrations. Mount failures are logged but never abort server start —
// a misbehaving plugin must not prevent the main service from serving
// traffic.
package mount

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/plugin/eventbus"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	pkgplugin "github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/gin-gonic/gin"
)

// AuthMiddlewares bundles the host's auth handlers indexed by the
// AuthRequirement they satisfy. A nil entry means "requirement not supported
// in this build" — routes declaring it are logged and skipped.
type AuthMiddlewares struct {
	// User is applied to routes with Auth == plugin.AuthUser (JWT session).
	User gin.HandlerFunc
	// Admin is applied to routes with Auth == plugin.AuthAdmin.
	Admin gin.HandlerFunc
}

// MountRoutes walks every enabled plugin and registers its declared routes
// on the given gin engine. Main routes are already installed; plugin routes
// therefore share the global middleware chain (logger, CORS, etc.) plus the
// auth middleware matching RouteSpec.Auth.
func MountRoutes(ctx context.Context, router *gin.Engine, ldr *loader.Loader, auth AuthMiddlewares, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	states, err := ldr.ListStates(ctx)
	if err != nil {
		return err
	}
	for _, st := range states {
		if st.State != repository.PluginStateEnabled {
			continue
		}
		mountOne(router, st.ID, auth, log)
	}
	return nil
}

// mountOne registers every RouteSpec declared by a single enabled plugin.
func mountOne(router *gin.Engine, id string, auth AuthMiddlewares, log *slog.Logger) {
	p, ok := pkgplugin.Lookup(id)
	if !ok {
		log.Warn("plugin registered but not compiled in; skipping routes", "plugin", id)
		return
	}
	for _, route := range p.Meta().Routes {
		registerRoute(router, id, route, auth, log)
	}
}

// registerRoute resolves the middleware chain and attaches a single route.
func registerRoute(router *gin.Engine, id string, route pkgplugin.RouteSpec, auth AuthMiddlewares, log *slog.Logger) {
	if route.Handler == nil {
		log.Error("plugin route skipped: nil handler",
			"plugin", id, "method", route.Method, "path", route.Path)
		return
	}
	handlers, ok := buildHandlers(route, auth)
	if !ok {
		log.Error("plugin route skipped: auth middleware unavailable",
			"plugin", id, "method", route.Method, "path", route.Path, "auth", route.Auth)
		return
	}
	router.Handle(route.Method, route.Path, handlers...)
	log.Info("plugin route mounted",
		"plugin", id, "method", route.Method, "path", route.Path, "auth", route.Auth)
}

// buildHandlers returns the ordered handler chain for a route, or ok=false
// when the auth requirement is unknown / unsupported.
func buildHandlers(route pkgplugin.RouteSpec, auth AuthMiddlewares) ([]gin.HandlerFunc, bool) {
	switch route.Auth {
	case pkgplugin.AuthNone:
		return []gin.HandlerFunc{route.Handler}, true
	case pkgplugin.AuthUser:
		if auth.User == nil {
			return nil, false
		}
		return []gin.HandlerFunc{auth.User, route.Handler}, true
	case pkgplugin.AuthAdmin:
		if auth.Admin == nil {
			return nil, false
		}
		return []gin.HandlerFunc{auth.Admin, route.Handler}, true
	default:
		return nil, false
	}
}

// MountSubscriptions walks every enabled plugin and registers its
// Meta.Subscribes entries on the event bus. Individual subscription errors
// are logged per-topic so one bad topic cannot silently drop its siblings.
func MountSubscriptions(ctx context.Context, bus *eventbus.Bus, ldr *loader.Loader, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	states, err := ldr.ListStates(ctx)
	if err != nil {
		return err
	}
	for _, st := range states {
		if st.State != repository.PluginStateEnabled {
			continue
		}
		subscribeOne(bus, st.ID, log)
	}
	return nil
}

// subscribeOne registers every EventSubscription declared by the plugin.
func subscribeOne(bus *eventbus.Bus, id string, log *slog.Logger) {
	p, ok := pkgplugin.Lookup(id)
	if !ok {
		log.Warn("plugin registered but not compiled in; skipping subscriptions", "plugin", id)
		return
	}
	for _, sub := range p.Meta().Subscribes {
		if err := bus.Subscribe(sub); err != nil {
			log.Error("plugin subscription failed",
				"plugin", id, "topic", sub.Topic, "kind", sub.Kind, "error", err)
			continue
		}
		log.Info("plugin subscription mounted",
			"plugin", id, "topic", sub.Topic, "kind", sub.Kind, "tag", sub.SubscriberTag)
	}
}
