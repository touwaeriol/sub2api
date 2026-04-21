/**
 * Barrel re-export for the channel form module. The implementation has
 * been split into topic-focused files to keep each under the 200-line
 * TypeScript cap (CLAUDE.md §14):
 *
 *   - channelFormTypes.ts        — shared interfaces + PLATFORM_ORDER + tag class
 *   - pricingConversion.ts       — $/MTok ↔ per-token converters, interval conversion
 *   - pricingValidation.ts       — model-pattern conflict + interval sanity checks
 *   - platformSections.ts        — Channel ↔ PlatformSection[] round-trip
 *   - paramOverrideValidation.ts — pre-submit rule validation
 *
 * This file exists so every historic `from './types'` import keeps working
 * unchanged. When adding new functionality, put it in the appropriate
 * topic file rather than expanding this re-export block.
 */

export * from './channelFormTypes'
export * from './pricingConversion'
export * from './pricingValidation'
export * from './platformSections'
export * from './paramOverrideValidation'
