// Package plugin defines the contracts that sub2api plugins implement and
// the CoreAPI surface that the host exposes to them.
//
// # Architecture
//
// Plugins are ordinary Go packages compiled into the main binary. A plugin
// registers itself in its init function via [Register]; the host then loads,
// initializes and activates the plugin according to its [Meta]. There is no
// dynamic loading — enabling or disabling a plugin is a runtime switch over
// already-linked code, changing versions requires rebuilding the binary.
//
// # Lifecycle
//
// Each plugin moves through the following states in the `plugins` table:
//
//		NotInstalled -> Installed -> Disabled <-> Enabled
//
//	  - NotInstalled: the plugin is linked but has no row in `plugins`.
//	  - Installed: the host has recorded the plugin, applied its declared
//	    schema and migrations, but not activated it.
//	  - Enabled / Disabled: runtime toggle; transitioning calls Start/Shutdown.
//	  - Uninstall removes the plugins row; --purge additionally drops the
//	    plugin's declared tables in reverse order.
//
// # Boundary
//
// This package is the import boundary between plugin authors and the host.
// Plugins MUST NOT import packages under github.com/Wei-Shaw/sub2api/internal.
// The host MUST NOT depend on a specific plugin's concrete types — cross-plugin
// access goes through Meta.Exports plus [PluginAs].
//
// # Versioning
//
// Plugins declare Meta.APIVersion (semver). The host checks it against
// [SDKVersion] using [CheckAPIVersion]. Major mismatch or sdk minor below the
// required value causes the plugin to be rejected at load time.
package plugin
