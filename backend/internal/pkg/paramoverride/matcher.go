package paramoverride

// Match returns the compiled rules that apply to the given (platform, model)
// pair, in their original input order. The returned slice is nil when no
// rules are configured for the platform or none of the configured rules
// match; callers must not mutate it (shared with the snapshot).
func (c *Compiled) Match(platform, model string) []CompiledRule {
	if c == nil {
		return nil
	}
	rules := c.byPlatform[platform]
	if len(rules) == 0 {
		return nil
	}
	out := make([]CompiledRule, 0, len(rules))
	for i := range rules {
		if rules[i].MatchesModel(model) {
			out = append(out, rules[i])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
