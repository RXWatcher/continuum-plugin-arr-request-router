package routing_test

import (
	"encoding/json"
	"testing"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/routing"
)

func mkRule(field, op string, value string) routing.Rule {
	return routing.Rule{Field: field, Op: op, Value: json.RawMessage(value)}
}

func TestEvaluateEmptyGroupsMatchesEverything(t *testing.T) {
	matched, groups := routing.Evaluate(
		routing.Rules{Match: routing.MatchAll},
		routing.Context{Event: routing.RequestEvent{MediaType: "movie", Year: 2003}},
	)
	if !matched {
		t.Fatal("expected match on empty groups")
	}
	if groups != nil {
		t.Fatalf("expected nil groups, got %+v", groups)
	}
}

func TestEvaluateAllMatchAllRules(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{
				mkRule("year", "gte", `2000`),
				mkRule("mediaType", "eq", `"movie"`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{MediaType: "movie", Year: 2003}}
	matched, groups := routing.Evaluate(rules, ctx)
	if !matched {
		t.Fatal("expected match")
	}
	if len(groups) != 1 || !groups[0].Matched {
		t.Fatalf("group not matched: %+v", groups)
	}
	if len(groups[0].Rules) != 2 {
		t.Fatalf("rule count: %+v", groups[0].Rules)
	}
	for _, r := range groups[0].Rules {
		if !r.Matched {
			t.Errorf("expected rule match: %+v", r)
		}
	}
}

func TestEvaluateAllMatchOneFalse(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{
				mkRule("year", "gte", `2000`),
				mkRule("year", "lt", `2002`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{MediaType: "movie", Year: 2003}}
	matched, groups := routing.Evaluate(rules, ctx)
	if matched {
		t.Fatal("expected no match (year=2003 is not < 2002)")
	}
	// First rule matches, second doesn't — both must be in trace
	if !groups[0].Rules[0].Matched {
		t.Error("rule 0 should match")
	}
	if groups[0].Rules[1].Matched {
		t.Error("rule 1 should NOT match")
	}
}

func TestEvaluateAnyMatchOneTrue(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAny, Rules: []routing.Rule{
				mkRule("year", "lt", `2000`),
				mkRule("year", "gte", `2000`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{Year: 2003}}
	matched, groups := routing.Evaluate(rules, ctx)
	if !matched {
		t.Fatal("any-group should match because at least one rule does")
	}
	if !groups[0].Matched {
		t.Error("group.Matched should be true")
	}
}

func TestEvaluateMissingFieldYieldsRuleFalseWithNote(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{
				// genres is GroupB; ctx has no Primary so it's not loaded.
				mkRule("genres", "contains", `"Animation"`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{MediaType: "movie", Year: 2003}}
	matched, groups := routing.Evaluate(rules, ctx)
	if matched {
		t.Fatal("expected no match")
	}
	rr := groups[0].Rules[0]
	if rr.Matched {
		t.Error("rule.Matched should be false")
	}
	if rr.Note != "missing" {
		t.Errorf("expected Note=missing, got %q", rr.Note)
	}
}

func TestEvaluateInvalidRegexYieldsRuleFalseWithNote(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{
				mkRule("title", "regex", `"["`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{Title: "anything"}}
	matched, groups := routing.Evaluate(rules, ctx)
	if matched {
		t.Fatal("expected no match (invalid regex)")
	}
	rr := groups[0].Rules[0]
	if rr.Matched {
		t.Error("rule.Matched should be false")
	}
	if rr.Note == "" {
		t.Error("expected non-empty note for invalid regex")
	}
}

func TestEvaluateTopLevelAllRequiresEveryGroup(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{mkRule("year", "gte", `2000`)}},
			{Match: routing.MatchAll, Rules: []routing.Rule{mkRule("year", "lt", `2002`)}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{Year: 2003}}
	// Group 1 matches (year>=2000), Group 2 doesn't (year not <2002), top-level all => false.
	matched, _ := routing.Evaluate(rules, ctx)
	if matched {
		t.Fatal("expected top-level all to be false when one group fails")
	}
}

func TestEvaluateTopLevelAnyMatchesIfOneGroupMatches(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAny,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{mkRule("year", "lt", `1999`)}},
			{Match: routing.MatchAll, Rules: []routing.Rule{mkRule("year", "gte", `2000`)}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{Year: 2003}}
	matched, _ := routing.Evaluate(rules, ctx)
	if !matched {
		t.Fatal("expected top-level any to match because second group does")
	}
}

func TestEvaluateRulesNotShortCircuited(t *testing.T) {
	// 'any' group with 3 rules — the third one passing alone should not
	// cause the first two to be skipped from the trace.
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAny, Rules: []routing.Rule{
				mkRule("year", "lt", `1999`),
				mkRule("year", "lt", `2000`),
				mkRule("year", "lt", `2010`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{Year: 2003}}
	matched, groups := routing.Evaluate(rules, ctx)
	if !matched {
		t.Fatal("expected match")
	}
	if len(groups[0].Rules) != 3 {
		t.Fatalf("expected 3 rule results, got %d", len(groups[0].Rules))
	}
}

func TestEvaluatePropagatesNote_TypeMismatch(t *testing.T) {
	rules := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{
			{Match: routing.MatchAll, Rules: []routing.Rule{
				// gt with a string value vs a string field actual => type mismatch
				mkRule("title", "gt", `5`),
			}},
		},
	}
	ctx := routing.Context{Event: routing.RequestEvent{Title: "x"}}
	matched, groups := routing.Evaluate(rules, ctx)
	if matched {
		t.Fatal("expected no match")
	}
	rr := groups[0].Rules[0]
	if rr.Note == "" {
		t.Error("expected non-empty note for type mismatch")
	}
}
