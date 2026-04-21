package paramoverride

import "errors"

// Sentinel errors returned by Compile. All carry lower-case messages; wrap
// them with fmt.Errorf at the call site when additional context is needed.
var (
	ErrInvalidTarget      = errors.New("invalid target")
	ErrInvalidAction      = errors.New("invalid action")
	ErrAppendOnBody       = errors.New("append action is only valid for header target")
	ErrValueRequired      = errors.New("value is required for set/merge/append actions")
	ErrValueNullForbidden = errors.New("value cannot be null for set/merge/append actions; use the remove action to delete a field")
	ErrPathRequired       = errors.New("path is required")
	ErrPathTooLong        = errors.New("path exceeds max length")
	ErrGlobTooLong        = errors.New("model_glob exceeds max length")
	ErrInvalidGlob        = errors.New("invalid model_glob pattern")
	ErrTooManyRules       = errors.New("too many rules for a single platform")
	ErrInvalidJSONBody    = errors.New("invalid JSON body")
	ErrInvalidValue       = errors.New("invalid JSON value")
)
