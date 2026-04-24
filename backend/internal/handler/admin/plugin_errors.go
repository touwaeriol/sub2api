package admin

// Plugin admin error codes. Keep ranges distinct from other admin handlers.
// 42xxx is reserved for plugin lifecycle / dead-letter errors.
const (
	// ErrCodePluginNotFound — requested plugin id is unknown to the host.
	ErrCodePluginNotFound = 42001
	// ErrCodePluginInvalidState — lifecycle transition not allowed in current state.
	ErrCodePluginInvalidState = 42002
	// ErrCodePluginLifecycle — lifecycle operation (install/enable/...) failed.
	ErrCodePluginLifecycle = 42003
	// ErrCodePluginInternal — unexpected repository/loader failure.
	ErrCodePluginInternal = 42004
	// ErrCodePluginBadRequest — malformed query or body.
	ErrCodePluginBadRequest = 42005

	// ErrCodeDeadLetterNotFound — dead-letter id not found.
	ErrCodeDeadLetterNotFound = 42101
	// ErrCodeDeadLetterListFailed — listing dead letters failed.
	ErrCodeDeadLetterListFailed = 42102
	// ErrCodeDeadLetterRetryFailed — retry invocation failed.
	ErrCodeDeadLetterRetryFailed = 42103
	// ErrCodeDeadLetterDeleteFailed — delete failed.
	ErrCodeDeadLetterDeleteFailed = 42104
)

// Keys used in error response `details` maps.
const (
	ErrDetailPluginID      = "plugin_id"
	ErrDetailState         = "state"
	ErrDetailDeadLetterID  = "dead_letter_id"
	ErrDetailTopic         = "topic"
	ErrDetailSubscriberTag = "subscriber_tag"
)

// Bounds for list endpoints.
const (
	defaultDeadLetterPageSize = 50
	maxDeadLetterPageSize     = 200
	defaultDeadLetterPage     = 1
)
