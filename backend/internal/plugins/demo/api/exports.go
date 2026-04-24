// Package demoapi is the public export surface of the demo plugin. Other
// plugins reach into these types via plugin.PluginAs[Exports].
//
// This package intentionally has no dependency on the demo's internal
// packages (including its ent client); it is pure interfaces + DTOs so
// cross-plugin callers do not drag the entire demo object graph into
// their import set.
package demoapi

import "context"

// Exports is the cross-plugin API the demo plugin exposes through
// Meta.Exports. Retrieve it with:
//
//	exp, err := plugin.PluginAs[demoapi.Exports](core, demo.PluginID)
type Exports interface {
	// LatestNote returns the most recent audit note for accountID, or
	// (nil, nil) when no note exists.
	LatestNote(ctx context.Context, accountID int64) (*NoteDTO, error)
}

// NoteDTO is a plugin-to-plugin snapshot of a note row. Field names and
// types are the stable cross-plugin contract — changing them is a
// breaking change (treat like a Meta.Exports type bump).
type NoteDTO struct {
	ID        int64
	AccountID int64
	Content   string
}
