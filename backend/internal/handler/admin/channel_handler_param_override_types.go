package admin

import "encoding/json"

// paramOverrideRuleRequest mirrors service.ChannelParamOverrideRule for inbound
// admin API payloads. Validation is performed explicitly (see
// validateParamOverrideRule) because cross-field constraints (append only for
// header, value required unless action=remove) are not expressible with the
// struct tag validator.
type paramOverrideRuleRequest struct {
	Enabled     *bool           `json:"enabled"`
	ModelGlob   string          `json:"model_glob"`
	Target      string          `json:"target"`
	Action      string          `json:"action"`
	Path        string          `json:"path"`
	Value       json.RawMessage `json:"value,omitempty"`
	Description string          `json:"description,omitempty"`
}

// paramOverrideRuleResponse is the outbound counterpart. Enabled is returned
// as a concrete bool (not a pointer) because clients should always see the
// effective state.
type paramOverrideRuleResponse struct {
	Enabled     bool            `json:"enabled"`
	ModelGlob   string          `json:"model_glob"`
	Target      string          `json:"target"`
	Action      string          `json:"action"`
	Path        string          `json:"path"`
	Value       json.RawMessage `json:"value,omitempty"`
	Description string          `json:"description,omitempty"`
}

// paramOverrideLiteralNull is the JSON literal for `null`. A Set/Merge/Append
// rule with this value has no runtime effect distinct from "delete the
// field", and is almost always a user mistake (they forgot to switch the
// action to Remove). Reject it at admin time to force the explicit
// semantics.
var paramOverrideLiteralNull = []byte("null")

// paramOverrideReservedBodyPath is the body path callers are forbidden from
// overriding: rewriting `model` at the paramoverride layer would desync the
// billing record and the actual upstream model. Matches the frontend
// RESERVED_BODY_PATHS constant.
const paramOverrideReservedBodyPath = "model"
