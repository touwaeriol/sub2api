package admin

import (
	"bytes"
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
		converted, err := convertPlatformRules(platform, rules)
		if err != nil {
			return nil, err
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

// convertPlatformRules converts the rules for a single platform, enforcing
// the per-platform max count before falling through to per-rule validation.
// Extracted from paramOverridesRequestToService so the top-level function
// stays under the 30-line soft cap.
func convertPlatformRules(platform string, rules []paramOverrideRuleRequest) ([]service.ChannelParamOverrideRule, error) {
	if len(rules) > paramoverride.MaxRulesPerPlatform {
		return nil, infraerrors.BadRequest("PARAM_OVERRIDE_INVALID",
			"too many rules for platform").
			WithMetadata(map[string]string{
				"platform": platform,
				"count":    strconv.Itoa(len(rules)),
				"max":      strconv.Itoa(paramoverride.MaxRulesPerPlatform),
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
	return converted, nil
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
// Delegates to per-dimension checks so each stays focused and under the
// 30-line cap; order matches the user's mental model (target → action →
// path → value).
func validateParamOverrideRule(r service.ChannelParamOverrideRule) string {
	if reason := validateRuleTargetAction(r); reason != "" {
		return reason
	}
	if reason := validateRulePath(r); reason != "" {
		return reason
	}
	if reason := validateRuleGlob(r); reason != "" {
		return reason
	}
	return validateRuleValue(r)
}

// validateRuleTargetAction covers the target / action enum checks plus the
// two action×target combos that have no defined semantics.
func validateRuleTargetAction(r service.ChannelParamOverrideRule) string {
	switch r.Target {
	case paramoverride.TargetBody, paramoverride.TargetHeader:
	default:
		return paramOverrideReasonInvalidTarget
	}
	switch r.Action {
	case paramoverride.ActionSet, paramoverride.ActionMerge,
		paramoverride.ActionRemove, paramoverride.ActionAppend:
	default:
		return paramOverrideReasonInvalidAction
	}
	if r.Action == paramoverride.ActionAppend && r.Target != paramoverride.TargetHeader {
		return paramOverrideReasonAppendRequiresHeader
	}
	if r.Action == paramoverride.ActionMerge && r.Target == paramoverride.TargetHeader {
		return paramOverrideReasonMergeNotSupported
	}
	return ""
}

// validateRulePath covers the path-required / too-long / reserved-name
// checks. Reserved-path is body-only because header names can't collide with
// the model-routing key.
func validateRulePath(r service.ChannelParamOverrideRule) string {
	if r.Path == "" {
		return paramOverrideReasonPathRequired
	}
	if len(r.Path) > paramoverride.MaxPathLength {
		return paramOverrideReasonPathTooLong
	}
	if r.Target == paramoverride.TargetBody && r.Path == paramOverrideReservedBodyPath {
		return paramOverrideReasonPathModelReserved
	}
	return ""
}

// validateRuleGlob enforces the model_glob length cap. Empty glob is
// permitted (treated as "match all" by the compiler).
func validateRuleGlob(r service.ChannelParamOverrideRule) string {
	if len(r.ModelGlob) > paramoverride.MaxModelGlobLength {
		return paramOverrideReasonGlobTooLong
	}
	return ""
}

// validateRuleValue enforces the "value is required unless Remove, and is
// never literal null" contract. Remove rules ignore the value slot entirely.
func validateRuleValue(r service.ChannelParamOverrideRule) string {
	if r.Action == paramoverride.ActionRemove {
		return ""
	}
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
