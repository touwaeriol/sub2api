package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// validateChannelConfig 校验渠道的定价和映射配置（冲突检测 + 区间校验 + 计费模式校验）。
// Create 和 Update 共用此函数，避免重复。
func validateChannelConfig(pricing []ChannelModelPricing, mapping map[string]map[string]string) error {
	if err := validateNoConflictingModels(pricing); err != nil {
		return err
	}
	if err := validatePricingIntervals(pricing); err != nil {
		return err
	}
	if err := validateNoConflictingMappings(mapping); err != nil {
		return err
	}
	return validatePricingBillingMode(pricing)
}

// validatePricingBillingMode 校验计费模式配置：按次/图片模式必须配价格或区间，所有价格字段不能为负，区间至少有一个价格字段。
func validatePricingBillingMode(pricing []ChannelModelPricing) error {
	for _, p := range pricing {
		if err := checkBillingModeRequirements(p); err != nil {
			return err
		}
		if err := checkPricesNotNegative(p); err != nil {
			return err
		}
		if err := checkIntervalsHavePrices(p); err != nil {
			return err
		}
	}
	return nil
}

func checkBillingModeRequirements(p ChannelModelPricing) error {
	if p.BillingMode == BillingModePerRequest || p.BillingMode == BillingModeImage {
		if p.PerRequestPrice == nil && len(p.Intervals) == 0 {
			return infraerrors.BadRequest(
				"BILLING_MODE_MISSING_PRICE",
				"per-request price or intervals required for per_request/image billing mode",
			)
		}
	}
	return nil
}

func checkPricesNotNegative(p ChannelModelPricing) error {
	checks := []struct {
		field string
		val   *float64
	}{
		{"input_price", p.InputPrice},
		{"output_price", p.OutputPrice},
		{"cache_write_price", p.CacheWritePrice},
		{"cache_read_price", p.CacheReadPrice},
		{"image_output_price", p.ImageOutputPrice},
		{"per_request_price", p.PerRequestPrice},
	}
	for _, c := range checks {
		if c.val != nil && *c.val < 0 {
			return infraerrors.BadRequest("NEGATIVE_PRICE", fmt.Sprintf("%s must be >= 0", c.field))
		}
	}
	return nil
}

func checkIntervalsHavePrices(p ChannelModelPricing) error {
	for _, iv := range p.Intervals {
		if iv.InputPrice == nil && iv.OutputPrice == nil &&
			iv.CacheWritePrice == nil && iv.CacheReadPrice == nil &&
			iv.PerRequestPrice == nil {
			return infraerrors.BadRequest(
				"INTERVAL_MISSING_PRICE",
				fmt.Sprintf("interval [%d, %s] has no price fields set for model %v",
					iv.MinTokens, formatMaxTokens(iv.MaxTokens), p.Models),
			)
		}
	}
	return nil
}

func formatMaxTokens(max *int) string {
	if max == nil {
		return "∞"
	}
	return fmt.Sprintf("%d", *max)
}

// modelEntry 表示一个模型模式条目（用于冲突检测）
type modelEntry struct {
	pattern  string // 原始模式（如 "claude-*" 或 "claude-opus-4"）
	prefix   string // lowercase 前缀（通配符去掉 *，精确名保持原样）
	wildcard bool
}

// conflictsBetween 检查两个模型模式是否冲突
func conflictsBetween(a, b modelEntry) bool {
	switch {
	case !a.wildcard && !b.wildcard:
		return a.prefix == b.prefix
	case a.wildcard && !b.wildcard:
		return strings.HasPrefix(b.prefix, a.prefix)
	case !a.wildcard && b.wildcard:
		return strings.HasPrefix(a.prefix, b.prefix)
	default:
		return strings.HasPrefix(a.prefix, b.prefix) ||
			strings.HasPrefix(b.prefix, a.prefix)
	}
}

// toModelEntry 将模型名转换为 modelEntry
func toModelEntry(pattern string) modelEntry {
	lower := strings.ToLower(pattern)
	isWild := strings.HasSuffix(lower, "*")
	prefix := lower
	if isWild {
		prefix = strings.TrimSuffix(lower, "*")
	}
	return modelEntry{pattern: pattern, prefix: prefix, wildcard: isWild}
}

// validateNoConflictingModels 检查定价列表中是否有冲突模型模式（同一平台下）。
// 冲突包括：精确重复、通配符之间的前缀包含、通配符与精确名的前缀匹配。
func validateNoConflictingModels(pricingList []ChannelModelPricing) error {
	byPlatform := make(map[string][]modelEntry)
	for _, p := range pricingList {
		for _, model := range p.Models {
			byPlatform[p.Platform] = append(byPlatform[p.Platform], toModelEntry(model))
		}
	}
	for platform, entries := range byPlatform {
		if err := detectConflicts(entries, platform, "MODEL_PATTERN_CONFLICT", "model patterns"); err != nil {
			return err
		}
	}
	return nil
}

// validateNoConflictingMappings 检查模型映射中是否有冲突的源模式
func validateNoConflictingMappings(mapping map[string]map[string]string) error {
	for platform, platformMapping := range mapping {
		entries := make([]modelEntry, 0, len(platformMapping))
		for src := range platformMapping {
			entries = append(entries, toModelEntry(src))
		}
		if err := detectConflicts(entries, platform, "MAPPING_PATTERN_CONFLICT", "mapping source patterns"); err != nil {
			return err
		}
	}
	return nil
}

func validatePricingIntervals(pricingList []ChannelModelPricing) error {
	for _, pricing := range pricingList {
		if err := ValidateIntervals(pricing.Intervals); err != nil {
			return infraerrors.BadRequest(
				"INVALID_PRICING_INTERVALS",
				fmt.Sprintf("invalid pricing intervals for platform '%s' models %v: %v",
					pricing.Platform, pricing.Models, err),
			)
		}
	}
	return nil
}

// detectConflicts 在一组 modelEntry 中检测冲突，返回带有 errCode 和 label 的错误
func detectConflicts(entries []modelEntry, platform, errCode, label string) error {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if conflictsBetween(entries[i], entries[j]) {
				return infraerrors.BadRequest(errCode,
					fmt.Sprintf("%s '%s' and '%s' conflict in platform '%s': overlapping match range",
						label, entries[i].pattern, entries[j].pattern, platform))
			}
		}
	}
	return nil
}
