//go:build unit

package api

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

func TestFactory_EmptyDeps_ReturnsUnimplementedEverywhere(t *testing.T) {
	factory := NewCoreAPIFactory(Dependencies{})
	core := factory.For("demo", []plugin.Permission{plugin.PermAccountRead, plugin.PermBillingWrite})

	if core.PluginID() != "demo" {
		t.Fatalf("PluginID wrong: %s", core.PluginID())
	}

	if _, err := core.Accounts().Find(context.Background(), 1); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Accounts.Find expected ErrNotImplemented, got %v", err)
	}
	if _, err := core.Billing().ListSupportedModels(context.Background()); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Billing.ListSupportedModels expected ErrNotImplemented, got %v", err)
	}
	if _, err := core.Users().Find(context.Background(), 1); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Users.Find expected ErrNotImplemented, got %v", err)
	}
	if _, err := core.Orders().Find(context.Background(), "x"); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Orders.Find expected ErrNotImplemented, got %v", err)
	}
	if _, err := core.Scheduler().Snapshot(context.Background()); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Scheduler.Snapshot expected ErrNotImplemented, got %v", err)
	}
	if _, err := core.Crypto().Encrypt(nil); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Crypto.Encrypt expected ErrNotImplemented, got %v", err)
	}
	if _, err := core.Settings().Get(context.Background(), "k"); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("Settings.Get expected ErrNotImplemented, got %v", err)
	}
}

func TestFactory_LoggerCarriesPluginTag(t *testing.T) {
	factory := NewCoreAPIFactory(Dependencies{})
	core := factory.For("tagged", nil)
	if core.Logger() == nil {
		t.Fatal("Logger should not be nil")
	}
	// smoke: ensure every log level can be called without panic.
	core.Logger().Debug("x", "k", 1)
	core.Logger().Info("x")
	core.Logger().Warn("x")
	core.Logger().Error("x")
}

func TestFactory_PluginsLookupUsesSDKRegistry(t *testing.T) {
	factory := NewCoreAPIFactory(Dependencies{})
	core := factory.For("lookup-probe", nil)
	if _, ok := core.Plugins().Lookup("does-not-exist"); ok {
		t.Fatal("expected unknown id to miss")
	}
}
