package api

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// billingAPI implements plugin.BillingAPI on top of the existing
// service.BillingService (pricing, model catalogue) and
// service.UsageBillingRepository (usage record persistence).
type billingAPI struct {
	guard       *guard
	billing     *service.BillingService
	usageRepo   service.UsageBillingRepository
	accountRepo service.AccountRepository
}

// newBillingAPI wires the host services; when BillingService is nil the
// returned instance returns plugin.ErrNotImplemented for every method.
func newBillingAPI(c *coreAPIImpl) plugin.BillingAPI {
	if c.deps.BillingService == nil {
		return unimplementedBillingAPI{}
	}
	return &billingAPI{
		guard:       c.guard,
		billing:     c.deps.BillingService,
		usageRepo:   c.deps.UsageBillingRepo,
		accountRepo: c.deps.AccountRepo,
	}
}

// GetModelPricing returns the pricing snapshot for model, projecting the
// service-layer ModelPricing into the plugin-facing DTO to keep internal
// fields (priority tiers, long-context multipliers) out of the contract.
func (b *billingAPI) GetModelPricing(ctx context.Context, model string) (*plugin.ModelPricing, error) {
	if err := b.guard.requirePerm(plugin.PermBillingWrite); err != nil {
		return nil, err
	}
	pricing, err := b.billing.GetModelPricing(model)
	if err != nil {
		return nil, fmt.Errorf("plugin billing get pricing: %w", err)
	}
	return toPricingDTO(pricing), nil
}

// ListSupportedModels returns every model id known to the billing service.
func (b *billingAPI) ListSupportedModels(ctx context.Context) ([]string, error) {
	if err := b.guard.requirePerm(plugin.PermBillingWrite); err != nil {
		return nil, err
	}
	return b.billing.ListSupportedModels(), nil
}

// Record hands a UsageRecord to UsageBillingRepository.Apply. RequestID is
// mandatory for idempotency; the host service treats a missing id as a
// programming error.
func (b *billingAPI) Record(ctx context.Context, record plugin.UsageRecord) error {
	if err := b.guard.requirePerm(plugin.PermBillingWrite); err != nil {
		return err
	}
	if b.usageRepo == nil {
		return fmt.Errorf("plugin billing record: %w", plugin.ErrNotImplemented)
	}
	cmd := toUsageBillingCommand(record)
	cmd.Normalize()
	if _, err := b.usageRepo.Apply(ctx, cmd); err != nil {
		return fmt.Errorf("plugin billing record apply: %w", err)
	}
	return nil
}

// toPricingDTO projects the internal *service.ModelPricing into the
// plugin-facing snapshot, discarding tier-specific fields.
func toPricingDTO(src *service.ModelPricing) *plugin.ModelPricing {
	if src == nil {
		return nil
	}
	return &plugin.ModelPricing{
		InputPricePerToken:         src.InputPricePerToken,
		OutputPricePerToken:        src.OutputPricePerToken,
		CacheCreationPricePerToken: src.CacheCreationPricePerToken,
		CacheReadPricePerToken:     src.CacheReadPricePerToken,
		ImageOutputPricePerToken:   src.ImageOutputPricePerToken,
	}
}

// toUsageBillingCommand converts the plugin-facing UsageRecord into the
// service-layer command used by UsageBillingRepository.Apply. Unknown
// dimensions in Extra are dropped — the current service schema has no
// free-form bucket.
func toUsageBillingCommand(r plugin.UsageRecord) *service.UsageBillingCommand {
	return &service.UsageBillingCommand{
		RequestID:           r.RequestID,
		UserID:              r.UserID,
		AccountID:           r.AccountID,
		Model:               r.Model,
		InputTokens:         int(r.Usage.InputTokens),
		OutputTokens:        int(r.Usage.OutputTokens),
		CacheReadTokens:     int(r.Usage.CachedInputTokens),
		CacheCreationTokens: 0,
	}
}

// unimplementedBillingAPI is returned when the host lacks a BillingService.
type unimplementedBillingAPI struct{}

func (unimplementedBillingAPI) Record(context.Context, plugin.UsageRecord) error {
	return plugin.ErrNotImplemented
}
func (unimplementedBillingAPI) GetModelPricing(context.Context, string) (*plugin.ModelPricing, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedBillingAPI) ListSupportedModels(context.Context) ([]string, error) {
	return nil, plugin.ErrNotImplemented
}
