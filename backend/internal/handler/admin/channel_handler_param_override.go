package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Param override validation reason codes. Strings are embedded in the
// structured error's metadata["reason"] and consumed verbatim by the
// frontend, so they must stay stable across releases and must not be
// localised.
const (
	paramOverrideReasonInvalidTarget        = "invalid_target"
	paramOverrideReasonInvalidAction        = "invalid_action"
	paramOverrideReasonAppendRequiresHeader = "append_requires_header_target"
	paramOverrideReasonMergeNotSupported    = "merge_not_supported_for_header"
	paramOverrideReasonPathRequired         = "path_required"
	paramOverrideReasonPathTooLong          = "path_too_long"
	paramOverrideReasonPathModelReserved    = "path_model_reserved"
	paramOverrideReasonGlobTooLong          = "model_glob_too_long"
	paramOverrideReasonValueRequired        = "value_required"
	paramOverrideReasonValueNullUseRemove   = "value_null_use_remove"
	paramOverrideReasonTooManyRules         = "too_many_rules"
	paramOverrideReasonCompileFailed        = "compile_failed"
)

// paramOverrideLiteralNull is the JSON literal for `null`. A Set/Merge/Append
// rule with this value has no runtime effect distinct from "delete the
// field", and is almost always a user mistake (they forgot to switch the
// action to Remove). Reject it at admin time to force the explicit
// semantics.
var paramOverrideLiteralNull = []byte("null")

// paramOverrideRuleRequest mirrors service.ChannelParamOverrideRule for inbound
// admin API payloads. Validation is performed explicitly (see
// validateParamOverrideRules) because cross-field constraints (append only for
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

// paramOverridesRequestToService converts the admin-API request map into the
// service-layer representation. The second return value carries a structured
// error when a rule fails validation; callers must surface it before the
// service call.
func paramOverridesRequestToService(req map[string][]paramOverrideRuleRequest) (service.ChannelParamOverrides, error) {
	if len(req) == 0 {
		return nil, nil
	}
	out := make(service.ChannelParamOverrides, len(req))
	for platform, rules := range req {
		if len(rules) > service.ParamOverrideMaxRulesPerPlatform {
			return nil, infraerrors.BadRequest("PARAM_OVERRIDE_INVALID",
				"too many rules for platform").
				WithMetadata(map[string]string{
					"platform": platform,
					"count":    strconv.Itoa(len(rules)),
					"max":      strconv.Itoa(service.ParamOverrideMaxRulesPerPlatform),
					"reason":   paramOverrideReasonTooManyRules,
				})
		}
		converted := make([]service.ChannelParamOverrideRule, 0, len(rules))
		for idx, r := range rules {
			svcRule, err := paramOverrideRuleRequestToService(platform, idx, r)
			if err != nil {
				return nil, err
			}
			converted = append(converted, svcRule)
		}
		out[platform] = converted
	}
	// Preflight paramoverride.Compile so users see any compile-time rejection
	// (invalid glob pattern, value that isn't legal JSON, etc.) at write time
	// instead of silently at request time via a runtime slog.Warn.
	if err := preflightCompileParamOverrides(out); err != nil {
		return nil, err
	}
	return out, nil
}

// preflightCompileParamOverrides runs paramoverride.Compile once per platform
// so every Compile-time rejection surfaces as a structured admin API error
// rather than a runtime slog.Warn that leaves the rule silently disabled.
func preflightCompileParamOverrides(overrides service.ChannelParamOverrides) error {
	for platform, rules := range overrides {
		pkgRules := make([]paramoverride.Rule, 0, len(rules))
		for _, r := range rules {
			pkgRules = append(pkgRules, r.ToParamOverrideRule())
		}
		if _, err := paramoverride.Compile(map[string][]paramoverride.Rule{platform: pkgRules}); err != nil {
			return infraerrors.BadRequest("PARAM_OVERRIDE_INVALID",
				fmt.Sprintf("failed to compile param overrides for platform %q: %s", platform, err.Error())).
				WithMetadata(map[string]string{
					"platform":      platform,
					"reason":        paramOverrideReasonCompileFailed,
					"compile_error": err.Error(),
				})
		}
	}
	return nil
}

// paramOverrideRuleRequestToService converts a single request rule, applying
// default Enabled=true semantics and validating the cross-field constraints.
func paramOverrideRuleRequestToService(platform string, idx int, r paramOverrideRuleRequest) (service.ChannelParamOverrideRule, error) {
	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	svcRule := service.ChannelParamOverrideRule{
		Enabled:     enabled,
		ModelGlob:   r.ModelGlob,
		Target:      r.Target,
		Action:      r.Action,
		Path:        r.Path,
		Value:       r.Value,
		Description: r.Description,
	}
	if reason := validateParamOverrideRule(svcRule); reason != "" {
		return service.ChannelParamOverrideRule{}, paramOverrideRuleError(platform, idx, reason)
	}
	return svcRule, nil
}

// validateParamOverrideRule checks the static shape of a single rule and
// returns an empty string when valid, or a short reason code otherwise.
// The reason is meant for the error details metadata; the full message is
// built by paramOverrideRuleError.
func validateParamOverrideRule(r service.ChannelParamOverrideRule) string {
	switch r.Target {
	case service.ParamOverrideTargetBody, service.ParamOverrideTargetHeader:
	default:
		return paramOverrideReasonInvalidTarget
	}
	switch r.Action {
	case service.ParamOverrideActionSet, service.ParamOverrideActionMerge,
		service.ParamOverrideActionRemove, service.ParamOverrideActionAppend:
	default:
		return paramOverrideReasonInvalidAction
	}
	if r.Action == service.ParamOverrideActionAppend && r.Target != service.ParamOverrideTargetHeader {
		return paramOverrideReasonAppendRequiresHeader
	}
	if r.Action == service.ParamOverrideActionMerge && r.Target == service.ParamOverrideTargetHeader {
		return paramOverrideReasonMergeNotSupported
	}
	if r.Path == "" {
		return paramOverrideReasonPathRequired
	}
	if len(r.Path) > service.ParamOverrideMaxPathLength {
		return paramOverrideReasonPathTooLong
	}
	if r.Target == service.ParamOverrideTargetBody && r.Path == "model" {
		return paramOverrideReasonPathModelReserved
	}
	if len(r.ModelGlob) > service.ParamOverrideMaxModelGlobLength {
		return paramOverrideReasonGlobTooLong
	}
	if r.Action != service.ParamOverrideActionRemove {
		if len(r.Value) == 0 {
			return paramOverrideReasonValueRequired
		}
		// Reject literal JSON null for non-remove actions. Set/Merge/Append
		// with null is almost always a mistake: the user meant to delete
		// the field and should use the Remove action instead. Matching
		// after TrimSpace so `  null  ` is also rejected.
		if bytes.Equal(bytes.TrimSpace(r.Value), paramOverrideLiteralNull) {
			return paramOverrideReasonValueNullUseRemove
		}
	}
	return ""
}

// paramOverrideRuleError returns a structured infraerror for a rejected rule.
func paramOverrideRuleError(platform string, idx int, reason string) error {
	return infraerrors.BadRequest("PARAM_OVERRIDE_INVALID",
		fmt.Sprintf("invalid param override rule (platform=%s, index=%d, reason=%s)", platform, idx, reason)).
		WithMetadata(map[string]string{
			"platform":   platform,
			"rule_index": strconv.Itoa(idx),
			"reason":     reason,
		})
}

// paramOverridesToResponse converts the service-layer override map into the
// outbound response type, substituting an empty map for nil so the JSON
// payload is always a well-formed object.
func paramOverridesToResponse(overrides service.ChannelParamOverrides) map[string][]paramOverrideRuleResponse {
	out := make(map[string][]paramOverrideRuleResponse, len(overrides))
	for platform, rules := range overrides {
		items := make([]paramOverrideRuleResponse, 0, len(rules))
		for _, r := range rules {
			items = append(items, paramOverrideRuleResponse{
				Enabled:     r.Enabled,
				ModelGlob:   r.ModelGlob,
				Target:      r.Target,
				Action:      r.Action,
				Path:        r.Path,
				Value:       r.Value,
				Description: r.Description,
			})
		}
		out[platform] = items
	}
	return out
}
