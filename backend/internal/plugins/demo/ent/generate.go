// Package ent hosts the generated ent client for the demo plugin. Running
// `go generate ./internal/plugins/demo/ent` re-emits the client under ./gen.
//
// The demo plugin has its own ent client (separate from the host's main
// ent package) so it can declare and own its tables without leaking schema
// responsibility into the host. The plugin SDK's SchemaProvider contract
// (pkg/plugin.NewEntSchemaProvider) expects exactly this shape.
package ent

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --target ./gen --feature sql/upsert,intercept,sql/execquery,sql/lock --idtype int64 ./schema
