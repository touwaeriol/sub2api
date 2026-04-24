package eventbus

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// Registry holds the EventSchema catalogue the bus validates publishes and
// subscriptions against. It is safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	schemas map[string]plugin.EventSchema
}

// NewRegistry returns an empty registry. Core topics should be loaded by the
// caller via [RegisterCoreSchemas].
func NewRegistry() *Registry {
	return &Registry{schemas: make(map[string]plugin.EventSchema)}
}

// Register adds a schema. Returns [plugin.ErrEventSchemaInvalid] for
// malformed input or [plugin.ErrEventSchemaDuplicate] when a different
// schema is already registered under the same topic. Registering the exact
// same schema twice is a no-op so plugins can re-declare core topics
// defensively.
func (r *Registry) Register(s plugin.EventSchema) error {
	if err := validateSchema(s); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.schemas[s.Topic]; ok {
		if schemasEqual(existing, s) {
			return nil
		}
		return fmt.Errorf("%w: topic=%s", plugin.ErrEventSchemaDuplicate, s.Topic)
	}
	r.schemas[s.Topic] = s
	return nil
}

// Get returns the registered schema for a topic; the second return reports
// whether the topic is known.
func (r *Registry) Get(topic string) (plugin.EventSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[topic]
	return s, ok
}

// Topics returns a snapshot of all registered topic names. Intended for
// admin tooling; ordering is not guaranteed.
func (r *Registry) Topics() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.schemas))
	for t := range r.schemas {
		out = append(out, t)
	}
	return out
}

// validateSchema enforces the non-negotiable fields every schema must have.
func validateSchema(s plugin.EventSchema) error {
	if s.Topic == "" {
		return fmt.Errorf("%w: empty topic", plugin.ErrEventSchemaInvalid)
	}
	if s.PayloadExample == nil {
		return fmt.Errorf("%w: topic=%s missing PayloadExample", plugin.ErrEventSchemaInvalid, s.Topic)
	}
	return nil
}

// schemasEqual compares two schemas by their observable identity so that
// idempotent re-registration is permitted.
func schemasEqual(a, b plugin.EventSchema) bool {
	if a.Topic != b.Topic || a.Kind != b.Kind {
		return false
	}
	return reflect.TypeOf(a.PayloadExample) == reflect.TypeOf(b.PayloadExample)
}
