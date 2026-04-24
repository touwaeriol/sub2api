package demo

import (
	"context"
	"fmt"

	demoapi "github.com/Wei-Shaw/sub2api/internal/plugins/demo/api"
	gen "github.com/Wei-Shaw/sub2api/internal/plugins/demo/ent/gen"
	"github.com/Wei-Shaw/sub2api/internal/plugins/demo/ent/gen/note"
)

// demoExports is the concrete implementation of demoapi.Exports returned
// through plugin.Meta.Exports. Peer plugins retrieve it with
// plugin.PluginAs[demoapi.Exports](core, demo.PluginID).
//
// The struct keeps a back-reference to the owning Plugin so the ent client
// pointer is always fresh (Init/Shutdown rebuild it on lifecycle changes).
type demoExports struct {
	owner  *Plugin
	client *gen.Client
}

// LatestNote returns the most recent audit note for accountID. The
// function compiles without a live ent client so static analysis passes
// even before the host wires a driver; at call time a nil client yields
// a typed error.
func (e *demoExports) LatestNote(ctx context.Context, accountID int64) (*demoapi.NoteDTO, error) {
	client := e.activeClient()
	if client == nil {
		return nil, fmt.Errorf("demo exports: ent client unavailable")
	}
	n, err := client.Note.Query().
		Where(note.AccountID(accountID)).
		Order(gen.Desc(note.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if gen.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("demo exports: query latest note: %w", err)
	}
	return &demoapi.NoteDTO{
		ID:        n.ID,
		AccountID: n.AccountID,
		Content:   n.Content,
	}, nil
}

// activeClient prefers the embedded pointer but falls back to the owner's
// current client; Init assigns both, Shutdown clears both. Tests can
// inject a client directly on the struct without constructing a Plugin.
func (e *demoExports) activeClient() *gen.Client {
	if e.client != nil {
		return e.client
	}
	if e.owner != nil {
		return e.owner.client
	}
	return nil
}
