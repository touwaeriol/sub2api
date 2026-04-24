package demo

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// onAccountCreated is the subscriber registered for
// plugin.TopicAccountCreated. The host delivers it a *plugin.Account
// payload (see eventbus.coreSchemas); we write one audit note per call
// so integration tests can observe the end-to-end flow.
//
// The EventSubscription.Handler shape is (ctx, payload any) error, so we
// type-assert and tolerate an unexpected payload by logging and dropping
// it (Notify topics deliver best-effort; errors are not retried).
func (p *Plugin) onAccountCreated(ctx context.Context, payload any) error {
	acct, ok := payload.(*plugin.Account)
	if !ok || acct == nil {
		p.core.Logger().Warn("demo: onAccountCreated got unexpected payload",
			"type", fmt.Sprintf("%T", payload))
		return nil
	}
	if p.client == nil {
		p.core.Logger().Warn("demo: skip note create (ent client unavailable)",
			"account_id", acct.ID)
		return nil
	}
	_, err := p.client.Note.Create().
		SetAccountID(acct.ID).
		SetContent(fmt.Sprintf("account created: platform=%s", acct.Platform)).
		Save(ctx)
	if err != nil {
		p.core.Logger().Error("demo: failed to create audit note",
			"account_id", acct.ID, "error", err)
		// Notify handlers report errors but they are not retried; we
		// return nil to avoid polluting bus metrics with transient faults.
		return nil
	}
	return nil
}
