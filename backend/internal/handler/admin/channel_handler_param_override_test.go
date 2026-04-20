//go:build unit

package admin

import (
	"encoding/json"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestValidateParamOverrideRule_AcceptsWellFormed(t *testing.T) {
	cases := []struct {
		name string
		rule service.ChannelParamOverrideRule
	}{
		{"body_set", service.ChannelParamOverrideRule{
			Target: service.ParamOverrideTargetBody, Action: service.ParamOverrideActionSet,
			Path: "thinking.budget_tokens", Value: json.RawMessage(`2048`),
		}},
		{"body_merge", service.ChannelParamOverrideRule{
			Target: service.ParamOverrideTargetBody, Action: service.ParamOverrideActionMerge,
			Path: "thinking", Value: json.RawMessage(`{"budget_tokens":2048}`),
		}},
		{"body_remove", service.ChannelParamOverrideRule{
			Target: service.ParamOverrideTargetBody, Action: service.ParamOverrideActionRemove,
			Path: "thinking",
		}},
		{"header_set", service.ChannelParamOverrideRule{
			Target: service.ParamOverrideTargetHeader, Action: service.ParamOverrideActionSet,
			Path: "X-Api-Version", Value: json.RawMessage(`"2024-10"`),
		}},
		{"header_append", service.ChannelParamOverrideRule{
			Target: service.ParamOverrideTargetHeader, Action: service.ParamOverrideActionAppend,
			Path: "Anthropic-Beta", Value: json.RawMessage(`"feature"`),
		}},
		{"header_remove", service.ChannelParamOverrideRule{
			Target: service.ParamOverrideTargetHeader, Action: service.ParamOverrideActionRemove,
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
		Target: service.ParamOverrideTargetHeader,
		Action: service.ParamOverrideActionMerge,
		Path:   "X-Foo",
		Value:  json.RawMessage(`{"x":1}`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != "merge_not_supported_for_header" {
		t.Fatalf("expected merge_not_supported_for_header, got %q", reason)
	}
}

func TestValidateParamOverrideRule_RejectsAppendBody(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: service.ParamOverrideTargetBody,
		Action: service.ParamOverrideActionAppend,
		Path:   "foo",
		Value:  json.RawMessage(`"bar"`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != "append_requires_header_target" {
		t.Fatalf("expected append_requires_header_target, got %q", reason)
	}
}

func TestValidateParamOverrideRule_ReservedModelPath(t *testing.T) {
	rule := service.ChannelParamOverrideRule{
		Target: service.ParamOverrideTargetBody,
		Action: service.ParamOverrideActionSet,
		Path:   "model",
		Value:  json.RawMessage(`"claude-x"`),
	}
	reason := validateParamOverrideRule(rule)
	if reason != "path_model_reserved" {
		t.Fatalf("expected path_model_reserved, got %q", reason)
	}
}

func TestParamOverridesRequestToService_BubblesMergeHeaderAsStructuredError(t *testing.T) {
	req := map[string][]paramOverrideRuleRequest{
		"anthropic": {
			{
				Target: service.ParamOverrideTargetHeader,
				Action: service.ParamOverrideActionMerge,
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
	if appErr.Metadata["reason"] != "merge_not_supported_for_header" {
		t.Fatalf("expected metadata reason merge_not_supported_for_header, got %q", appErr.Metadata["reason"])
	}
	if appErr.Metadata["platform"] != "anthropic" {
		t.Fatalf("expected metadata platform=anthropic, got %q", appErr.Metadata["platform"])
	}
	if appErr.Metadata["rule_index"] != "0" {
		t.Fatalf("expected metadata rule_index=0, got %q", appErr.Metadata["rule_index"])
	}
}
