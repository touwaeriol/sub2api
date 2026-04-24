//go:build unit

package eventbus

import (
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

func TestRegistryRegisterMissingTopic(t *testing.T) {
	r := NewRegistry()
	err := r.Register(plugin.EventSchema{PayloadExample: struct{}{}})
	if !errors.Is(err, plugin.ErrEventSchemaInvalid) {
		t.Fatalf("expected ErrEventSchemaInvalid, got %v", err)
	}
}

func TestRegistryRegisterNilPayload(t *testing.T) {
	r := NewRegistry()
	err := r.Register(plugin.EventSchema{Topic: "x"})
	if !errors.Is(err, plugin.ErrEventSchemaInvalid) {
		t.Fatalf("expected ErrEventSchemaInvalid, got %v", err)
	}
}

func TestRegistryDuplicateDiffersRejected(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(plugin.EventSchema{Topic: "x", Kind: plugin.EventKindNotify, PayloadExample: struct{}{}})
	err := r.Register(plugin.EventSchema{Topic: "x", Kind: plugin.EventKindSyncHook, PayloadExample: struct{}{}})
	if !errors.Is(err, plugin.ErrEventSchemaDuplicate) {
		t.Fatalf("expected ErrEventSchemaDuplicate, got %v", err)
	}
}

func TestRegistryIdenticalDuplicateAccepted(t *testing.T) {
	r := NewRegistry()
	s := plugin.EventSchema{Topic: "x", Kind: plugin.EventKindNotify, PayloadExample: struct{}{}}
	if err := r.Register(s); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(s); err != nil {
		t.Fatalf("re-register identical schema should succeed, got %v", err)
	}
}

func TestRegisterCoreSchemasPopulatesEight(t *testing.T) {
	r := NewRegistry()
	if err := RegisterCoreSchemas(r); err != nil {
		t.Fatalf("RegisterCoreSchemas: %v", err)
	}
	if got := len(r.Topics()); got != len(coreSchemas) {
		t.Fatalf("expected %d core topics, got %d", len(coreSchemas), got)
	}
	if _, ok := r.Get(plugin.TopicAccountBeforeDelete); !ok {
		t.Fatal("TopicAccountBeforeDelete not registered")
	}
}

func TestRegistryGetUnknownReturnsFalse(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("does.not.exist"); ok {
		t.Fatal("expected unknown topic to be missing")
	}
}
