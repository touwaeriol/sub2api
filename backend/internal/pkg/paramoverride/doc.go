// Package paramoverride provides compiled rule matching and application for
// channel-level parameter overrides.
//
// The package is intentionally decoupled from the service layer: callers convert
// their own rule representation into paramoverride.Rule before calling Compile.
// Compile returns an immutable snapshot that can be queried on the hot path
// with O(platform rule count) overhead and mutates request bodies/headers via
// sjson and net/http helpers.
package paramoverride
