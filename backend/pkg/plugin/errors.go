package plugin

import "errors"

// Standard errors returned by the SDK.
//
// Host and plugin code compare with [errors.Is]. Concrete implementations may
// wrap these sentinels with additional context using fmt.Errorf("%w: ...").
var (
	// ErrPermissionDenied is returned by CoreAPI calls when the caller plugin
	// lacks a required Permission in its Meta.Permissions.
	ErrPermissionDenied = errors.New("plugin: permission denied")

	// ErrInvalidTableName is returned by [AssertTableName] when a table does
	// not satisfy the plugin_<id>_* prefix rule.
	ErrInvalidTableName = errors.New("plugin: invalid table name")

	// ErrAPIVersionIncompat is returned by [CheckAPIVersion] when the plugin's
	// required APIVersion is incompatible with the host's [SDKVersion].
	ErrAPIVersionIncompat = errors.New("plugin: api version incompatible")

	// ErrCircularDependency is returned by the loader when the plugin
	// dependency graph contains a cycle.
	ErrCircularDependency = errors.New("plugin: circular dependency")

	// ErrPluginNotFound is returned by [PluginAs] when the requested plugin
	// id has not been registered.
	ErrPluginNotFound = errors.New("plugin: not found")

	// ErrExportsTypeMismatch is returned by [PluginAs] when the plugin's
	// Meta.Exports cannot be asserted to the requested type parameter.
	ErrExportsTypeMismatch = errors.New("plugin: exports type mismatch")

	// ErrDuplicateRegistration is returned by [Register] when two plugins
	// share the same id.
	ErrDuplicateRegistration = errors.New("plugin: duplicate registration")

	// ErrEventKindMismatch is returned by the event bus when a subscription
	// declares a different EventKind than the topic's schema.
	ErrEventKindMismatch = errors.New("plugin: event kind mismatch")

	// ErrEventTopicUnknown is returned by the event bus when a publish or
	// subscribe references a topic that has not been registered with an
	// [EventSchema].
	ErrEventTopicUnknown = errors.New("plugin: event topic unknown")

	// ErrEventHandlerSignature is returned by the event bus when a
	// subscription's handler is nil, or when a publish payload does not
	// match the topic's declared PayloadExample type.
	ErrEventHandlerSignature = errors.New("plugin: event handler signature mismatch")

	// ErrEventSchemaInvalid is returned by the registry when an [EventSchema]
	// is missing required fields (Topic, PayloadExample) at registration.
	ErrEventSchemaInvalid = errors.New("plugin: event schema invalid")

	// ErrEventSchemaDuplicate is returned by the registry when a topic is
	// registered more than once with diverging schemas.
	ErrEventSchemaDuplicate = errors.New("plugin: event schema duplicate")

	// ErrNotImplemented is returned by CoreAPI sub-interfaces whose minimum
	// viable implementation is not wired yet during Phase 0 scaffolding.
	// Plugins should check with [errors.Is] and gracefully skip the call.
	ErrNotImplemented = errors.New("plugin: not implemented")
)
