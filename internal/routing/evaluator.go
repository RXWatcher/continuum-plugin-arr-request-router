package routing

// Evaluate returns whether the rules match the context, plus a per-group
// trace. An empty Groups list matches everything (returns true with nil
// trace).
func Evaluate(rules Rules, ctx Context) (bool, []GroupResult) {
	if len(rules.Groups) == 0 {
		return true, nil
	}

	out := make([]GroupResult, 0, len(rules.Groups))

	var topMatched bool
	switch rules.Match {
	case MatchAny:
		topMatched = false
	default: // MatchAll (or anything else — validation should have caught bad combinator)
		topMatched = true
	}

	for _, g := range rules.Groups {
		gr := evalGroup(g, ctx)
		out = append(out, gr)
		switch rules.Match {
		case MatchAny:
			topMatched = topMatched || gr.Matched
		default:
			topMatched = topMatched && gr.Matched
		}
	}

	return topMatched, out
}

func evalGroup(g Group, ctx Context) GroupResult {
	out := GroupResult{
		Match: string(g.Match),
		Rules: make([]RuleResult, 0, len(g.Rules)),
	}

	var matched bool
	switch g.Match {
	case MatchAny:
		matched = false
	default:
		matched = true
	}

	// Iterate all rules without short-circuiting so the trace captures every result.
	for _, r := range g.Rules {
		rr := evalRule(r, ctx)
		out.Rules = append(out.Rules, rr)
		switch g.Match {
		case MatchAny:
			matched = matched || rr.Matched
		default:
			matched = matched && rr.Matched
		}
	}

	out.Matched = matched
	return out
}

func evalRule(r Rule, ctx Context) RuleResult {
	actual, ok := GetField(ctx, r.Field)
	if !ok {
		return RuleResult{Field: r.Field, Op: r.Op, Matched: false, Note: "missing"}
	}
	matched, note := Apply(r.Op, actual, r.Value)
	return RuleResult{Field: r.Field, Op: r.Op, Matched: matched, Note: note}
}
