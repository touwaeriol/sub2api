//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestRewriteUserID_EquivalenceAfterRefactor verifies the helper extraction
// (extractRewriteTarget + spliceUserID) preserves the early-return behavior
// shared by RewriteUserID and RewriteUserIDWithMasking. Each case is asserted
// against both methods (the masking variant is exercised via its sessionHash=""
// fallback, which delegates to RewriteUserID — keeping us honest that both
// paths use identical guards).
func TestRewriteUserID_EquivalenceAfterRefactor(t *testing.T) {
	const (
		accountUUID    = "11111111-2222-3333-4444-555555555555"
		clientID       = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
		fingerprintUA  = "claude-cli/2.1.112 (external, sdk-cli)"
		legacyValidUID = "user_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789_account_99999999-8888-7777-6666-555555555555_session_aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	)

	cases := []struct {
		name     string
		body     []byte
		expectEq bool // true = body should be returned unchanged
	}{
		{
			name:     "empty body",
			body:     nil,
			expectEq: true,
		},
		{
			name:     "metadata is null",
			body:     []byte(`{"metadata":null,"foo":"bar"}`),
			expectEq: true,
		},
		{
			name:     "metadata is array, not object",
			body:     []byte(`{"metadata":[1,2,3]}`),
			expectEq: true,
		},
		{
			name:     "user_id missing from metadata",
			body:     []byte(`{"metadata":{"other":"value"}}`),
			expectEq: true,
		},
		{
			name:     "valid legacy user_id rewritten",
			body:     []byte(`{"metadata":{"user_id":"` + legacyValidUID + `"}}`),
			expectEq: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewIdentityService(&identityCacheStub{})

			// Path 1: direct RewriteUserID
			gotDirect, err := svc.RewriteUserID(tc.body, 42, accountUUID, clientID, fingerprintUA)
			require.NoError(t, err)

			// Path 2: RewriteUserIDWithMasking with empty sessionHash → falls
			// back to RewriteUserID (same guard chain).
			account := &Account{ID: 42}
			gotMasking, err := svc.RewriteUserIDWithMasking(context.Background(), tc.body, account, accountUUID, clientID, fingerprintUA)
			require.NoError(t, err)

			if tc.expectEq {
				// Both methods must return body unchanged (same slice / same bytes).
				require.Equal(t, string(tc.body), string(gotDirect), "RewriteUserID should return body unchanged")
				require.Equal(t, string(tc.body), string(gotMasking), "RewriteUserIDWithMasking fallback should return body unchanged")
				return
			}

			// Rewrite path: metadata.user_id must change and remain a parseable
			// metadata user id with the expected device id + account uuid.
			for _, got := range [][]byte{gotDirect, gotMasking} {
				newUID := gjson.GetBytes(got, "metadata.user_id").String()
				require.NotEqual(t, legacyValidUID, newUID, "user_id must be rewritten")

				parsed := ParseMetadataUserID(newUID)
				require.NotNil(t, parsed, "rewritten user_id must be parseable")
				require.Equal(t, clientID, parsed.DeviceID)
				require.Equal(t, accountUUID, parsed.AccountUUID)
				require.NotEmpty(t, parsed.SessionID)
			}
		})
	}
}
