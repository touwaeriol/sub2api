//go:build unit

package admin

import (
	"encoding/json"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestValidateParamOverrideRule_AcceptsWellFormed(t *testing.T) {
	cases := []struct {
		name string
		rule service.ChannelParamOverrideRule
	}{
		{"body_set", service.ChannelParamOverrideRule{
			Target: paramoverride.TargetBody, Action: paramoverride.ActionSet,
			Path: "thinking.budget_tokens", Value: json.RawMessage(`2048`),
		}},
		{"body_merge", service.ChannelParamOverrideRule{
			Target: paramoverride.TargetBody, Action: paramoverride.ActionMerge,
			Path: "thinking", Value: json.RawMessage(`{"budget_tokens":2048}`),
		}},
		{"body_remove", service.ChannelParamOverrideRule{
			Target: paramoverride.TargetBody, Action: paramoverride.ActionRemove,
			Path: "thinking",
		}},
		{"header_set", service.ChannelParamOverrideRule{
			Target: paramoverride.TargetHeader, Action: paramoverride.ActionSet,
			Path: "X-Api-Version", Value: json.RawMessage(`"2024-10"`),
		}},
		{"header_append", service.ChannelParamOverrideRule{
			Target: paramoverride.TargetHeader, Action: paramoverride.ActionAppend,
			Path: "Anthropic-Beta", Value: json.RawMessage(`"feature"`),
		}},
		{"header_remove", service.ChannelParamOverrideRule{
			Target: paramoverride.TargetHeader, Action: paramoverride.ActionRemove,
			Path: "X-Extra",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateParamOverrideRule(tc.rule); got != "" {
				t.Fatalf("expected rule %+v to validate, got reason %q", tc.rule, got)
			}
		})
	}
}

func TestValidateParamOverrideRule_RejectsMergeHeader(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetHeader,
		Action: paramoverride.ActionMerge,
		Path:   "X-Foo",
		Value:  json.RawMessage(`{"x":1}`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonMergeNotSupported {
		t.Fatalf("expected %s, got %q", paramOverrideReasonMergeNotSupported, reason)
	}
}

func TestValidateParamOverrideRule_RejectsAppendBody(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetBody,
		Action: paramoverride.ActionAppend,
		Path:   "foo",
		Value:  json.RawMessage(`"bar"`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonAppendRequiresHeader {
		t.Fatalf("expected %s, got %q", paramOverrideReasonAppendRequiresHeader, reason)
	}
}

func TestValidateParamOverrideRule_ReservedModelPath(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetBody,
		Action: paramoverride.ActionSet,
		Path:   "model",
		Value:  json.RawMessage(`"claude-x"`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonPathModelReserved {
		t.Fatalf("expected %s, got %q", paramOverrideReasonPathModelReserved, reason)
	}
}

func TestValidateParamOverrideRule_RejectsNullValueForSet(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetBody,
		Action: paramoverride.ActionSet,
		Path:   "thinking.budget_tokens",
		Value:  json.RawMessage(`null`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonValueNullUseRemove {
		t.Fatalf("expected %s, got %q", paramOverrideReasonValueNullUseRemove, reason)
	}
}

func TestValidateParamOverrideRule_RejectsNullValueForMerge(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetBody,
		Action: paramoverride.ActionMerge,
		Path:   "thinking",
		Value:  json.RawMessage(`null`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonValueNullUseRemove {
		t.Fatalf("expected %s, got %q", paramOverrideReasonValueNullUseRemove, reason)
	}
}

func TestValidateParamOverrideRule_RejectsNullValueForAppend(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetHeader,
		Action: paramoverride.ActionAppend,
		Path:   "Anthropic-Beta",
		Value:  json.RawMessage(`null`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonValueNullUseRemove {
		t.Fatalf("expected %s, got %q", paramOverrideReasonValueNullUseRemove, reason)
	}
}

func TestValidateParamOverrideRule_RejectsNullValueWithWhitespace(t *testing.T) {
	// `  null  ` (surrounding whitespace) should also trip the guard.
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetBody,
		Action: paramoverride.ActionSet,
		Path:   "thinking.budget_tokens",
		Value:  json.RawMessage(`  null  `),
	}
	reason := validateParamOverrideRule(rule)
	if reason != paramOverrideReasonValueNullUseRemove {
		t.Fatalf("expected %s, got %q", paramOverrideReasonValueNullUseRemove, reason)
	}
}

func TestValidateParamOverrideRule_AllowsNullValueForRemove(t *testing.T) {
	// Remove actions ignore Value entirely, so null/empty/whatever all pass.
	rule := service.ChannelParamOverrideRule{
		Target: paramoverride.TargetBody,
		Action: paramoverride.ActionRemove,
		Path:   "thinking",
		Value:  json.RawMessage(`null`),
	}
	if reason := validateParamOverrideRule(rule); reason != "" {
		t.Fatalf("expected remove+null to validate, got reason %q", reason)
	}
}

func TestParamOverridesRequestToService_BubblesNullValueAsStructuredError(t *testing.T) {
	req := map[string][]paramOverrideRuleRequest{
		"openai": {
			{
				Target: paramoverride.TargetBody,
				Action: paramoverride.ActionSet,
				Path:   "reasoning.effort",
				Value:  json.RawMessage(`null`),
			},
		},
	}
	_, err := paramOverridesRequestToService(req)
	if err == nil {
		t.Fatalf("expected error for set+null value")
	}
	var appErr *infraerrors.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected ApplicationError, got %T: %v", err, err)
	}
	if appErr.Metadata["reason"] != paramOverrideReasonValueNullUseRemove {
		t.Fatalf("expected metadata reason %s, got %q", paramOverrideReasonValueNullUseRemove, appErr.Metadata["reason"])
	}
}

func TestParamOverridesRequestToService_BubblesMergeHeaderAsStructuredError(t *testing.T) {
	req := map[string][]paramOverrideRuleRequest{
		"anthropic": {
			{
				Target: paramoverride.TargetHeader,
				Action: paramoverride.ActionMerge,
				Path:   "X-Foo",
				Value:  json.RawMessage(`{"x":1}`),
			},
		},
	}
	_, err := paramOverridesRequestToService(req)
	if err == nil {
		t.Fatalf("expected error for merge+header")
	}
	var appErr *infraerrors.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected ApplicationError, got %T: %v", err, err)
	}
	if appErr.Reason != "PARAM_OVERRIDE_INVALID" {
		t.Fatalf("expected reason PARAM_OVERRIDE_INVALID, got %q", appErr.Reason)
	}
	if appErr.Metadata["reason"] != paramOverrideReasonMergeNotSupported {
		t.Fatalf("expected metadata reason %s, got %q", paramOverrideReasonMergeNotSupported, appErr.Metadata["reason"])
	}
	if appErr.Metadata["platform"] != "anthropic" {
		t.Fatalf("expected metadata platform=anthropic, got %q", appErr.Metadata["platform"])
	}
	if appErr.Metadata["rule_index"] != "0" {
		t.Fatalf("expected metadata rule_index=0, got %q", appErr.Metadata["rule_index"])
	}
}

// TestParamOverridesRequestToService_RejectsInvalidValueJsonOnCompile covers
// the main Compile-time failure mode the preflight exists to catch: the
// admin static shape check accepts any non-empty bytes for
// json.RawMessage, so a hand-crafted invalid JSON payload passes
// validateParamOverrideRule but trips Compile's classifyValue
// json.Unmarshal. Without the preflight this only surfaced at request
// time via slog.Warn, silently disabling the rule.
//
// (Glob compile failures are structurally impossible: compileGlob wraps
// every non-'*'/'?' char in regexp.QuoteMeta, so the resulting regex is
// always valid. The ValueRequired / PathRequired branches inside Compile
// are already pre-covered by validateParamOverrideRule in this package.)
func TestParamOverridesRequestToService_RejectsInvalidValueJsonOnCompile(t *testing.T) {
	req := map[string][]paramOverrideRuleRequest{
		"openai": {
			{
				Target: paramoverride.TargetBody,
				Action: paramoverride.ActionSet,
				Path:   "reasoning.effort",
				// {not json — passes the "len(Value)==0" static check but
				// classifyValue's json.Unmarshal will fail.
				Value: json.RawMessage(`{not json`),
			},
		},
	}
	_, err := paramOverridesRequestToService(req)
	if err == nil {
		t.Fatalf("expected error for invalid JSON value at compile time")
	}
	var appErr *infraerrors.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected ApplicationError, got %T: %v", err, err)
	}
	if appErr.Reason != "PARAM_OVERRIDE_INVALID" {
		t.Fatalf("expected reason PARAM_OVERRIDE_INVALID, got %q", appErr.Reason)
	}
	if appErr.Metadata["reason"] != paramOverrideReasonCompileFailed {
		t.Fatalf("expected metadata reason %s, got %q", paramOverrideReasonCompileFailed, appErr.Metadata["reason"])
	}
	if appErr.Metadata["platform"] != "openai" {
		t.Fatalf("expected metadata platform=openai, got %q", appErr.Metadata["platform"])
	}
	if appErr.Metadata["compile_error"] == "" {
		t.Fatalf("expected non-empty compile_error in metadata")
	}
}

// TestParamOverridesRequestToService_CompilesCleanRules sanity-checks that
// the preflight does not break valid rules — a full round-trip through
// validate + Compile must leave the converted map intact.
func TestParamOverridesRequestToService_CompilesCleanRules(t *testing.T) {
	req := map[string][]paramOverrideRuleRequest{
		"anthropic": {
			{
				ModelGlob: "claude-*",
				Target:    paramoverride.TargetBody,
				Action:    paramoverride.ActionSet,
				Path:      "thinking.budget_tokens",
				Value:     json.RawMessage(`2048`),
			},
		},
	}
	out, err := paramOverridesRequestToService(req)
	if err != nil {
		t.Fatalf("expected clean rule to pass, got %v", err)
	}
	if len(out["anthropic"]) != 1 {
		t.Fatalf("expected 1 converted rule, got %d", len(out["anthropic"]))
	}
}
