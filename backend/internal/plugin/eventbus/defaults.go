package eventbus

import (
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// coreSchemas enumerates the eight built-in topics declared in the plugin
// SDK. Kept in one place so the registry, docs and tests stay in sync.
var coreSchemas = []plugin.EventSchema{
	{
		Topic:          plugin.TopicAccountBeforeDelete,
		Kind:           plugin.EventKindSyncHook,
		PayloadExample: &plugin.Account{},
		Description:    "Fired before an account row is deleted; return an error to veto.",
	},
	{
		Topic:          plugin.TopicAccountCreated,
		Kind:           plugin.EventKindNotify,
		PayloadExample: &plugin.Account{},
		Description:    "Fired after an account row has been created.",
	},
	{
		Topic:          plugin.TopicAccountStateChanged,
		Kind:           plugin.EventKindNotify,
		PayloadExample: plugin.AccountStateChanged{},
		Description:    "Fired when an account's status transitions (e.g. active -> error).",
	},
	{
		Topic:          plugin.TopicRequestBeforeForward,
		Kind:           plugin.EventKindSyncHook,
		PayloadExample: &plugin.ForwardRequest{},
		Description:    "Fired before a gateway call is issued; plugins may rewrite or veto.",
	},
	{
		Topic:          plugin.TopicRequestAfterForward,
		Kind:           plugin.EventKindNotify,
		PayloadExample: &plugin.ForwardResult{},
		Description:    "Fired after a gateway call completes; used for usage reporting.",
	},
	{
		Topic:          plugin.TopicOrderPaid,
		Kind:           plugin.EventKindSyncHook,
		PayloadExample: plugin.OrderEvent{},
		Description:    "Fired inside the payment transaction; return an error to fail mark-paid.",
	},
	{
		Topic:          plugin.TopicOrderFulfilled,
		Kind:           plugin.EventKindNotify,
		PayloadExample: plugin.OrderEvent{},
		Description:    "Fired after an order is fully fulfilled (balance credit / subscription activated).",
	},
	{
		Topic:          plugin.TopicPluginSettingsChanged,
		Kind:           plugin.EventKindNotify,
		PayloadExample: plugin.SettingsChanged{},
		Description:    "Fired when a plugin's settings change via the admin UI.",
	},
}

// RegisterCoreSchemas pushes the built-in topics into the given registry.
// Safe to call multiple times — the registry accepts identical re-registration.
func RegisterCoreSchemas(r *Registry) error {
	for _, s := range coreSchemas {
		if err := r.Register(s); err != nil {
			return err
		}
	}
	return nil
}
