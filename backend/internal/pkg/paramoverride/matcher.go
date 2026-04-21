package paramoverride

// Match returns the compiled rules that apply to the given (platform, model)
// pair, in their original input order. The returned slice is nil when no
// rules are configured for the platform or none of the configured rules
// match; callers must not mutate it (shared with the snapshot).
//
// The output slice is allocated lazily so the common no-match case produces
// zero allocations — most requests run through a channel that either has no
// overrides for this platform or whose rules don't match the current model,
// so paying for a capacity-N make on every call was wasteful.
func (c *Compiled) Match(platform, model string) []CompiledRule {
	if c == nil {
		return nil
	}
	rules := c.byPlatform[platform]
	if len(rules) == 0 {
		return nil
	}
	var out []CompiledRule
	for i := range rules {
		if !rules[i].MatchesModel(model) {
			continue
		}
		if out == nil {
			out = make([]CompiledRule, 0, len(rules)-i)
		}
		out = append(out, rules[i])
	}
	return out
}
