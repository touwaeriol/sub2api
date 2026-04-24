//go:build unit

package api

import (
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

func TestGuard_RequirePerm_AllowsDeclared(t *testing.T) {
	g := newGuard("test-plugin", []plugin.Permission{plugin.PermAccountRead})
	if err := g.requirePerm(plugin.PermAccountRead); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestGuard_RequirePerm_DeniesUndeclared(t *testing.T) {
	g := newGuard("test-plugin", []plugin.Permission{plugin.PermAccountRead})
	err := g.requirePerm(plugin.PermAccountWrite)
	if err == nil {
		t.Fatal("expected denial, got nil")
	}
	if !errors.Is(err, plugin.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestGuard_RequirePerm_EmptyDenies(t *testing.T) {
	g := newGuard("empty", nil)
	err := g.requirePerm(plugin.PermAccountRead)
	if !errors.Is(err, plugin.ErrPermissionDenied) {
		t.Fatalf("expected denial, got %v", err)
	}
}

func TestGuard_RequireAny_AcceptsFirstMatch(t *testing.T) {
	g := newGuard("multi", []plugin.Permission{plugin.PermAccountWrite})
	if err := g.requireAny(plugin.PermAccountRead, plugin.PermAccountWrite); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestGuard_RequireAny_DeniesWhenNoneHeld(t *testing.T) {
	g := newGuard("multi", []plugin.Permission{plugin.PermCrypto})
	err := g.requireAny(plugin.PermAccountRead, plugin.PermAccountWrite)
	if !errors.Is(err, plugin.ErrPermissionDenied) {
		t.Fatalf("expected denial, got %v", err)
	}
}
