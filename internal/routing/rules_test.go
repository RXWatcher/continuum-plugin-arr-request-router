package routing_test

import (
	"encoding/json"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

func TestParseRulesValid(t *testing.T) {
	raw := []byte(`{"match":"all","groups":[{"match":"any","rules":[{"field":"year","op":"gte","value":2000}]}]}`)
	r, err := routing.ParseRules(raw)
	if err != nil { t.Fatal(err) }
	if r.Match != "all" || len(r.Groups) != 1 {
		t.Fatalf("unexpected: %+v", r)
	}
	if r.Groups[0].Match != "any" || len(r.Groups[0].Rules) != 1 {
		t.Fatalf("unexpected: %+v", r.Groups[0])
	}
	if r.Groups[0].Rules[0].Field != "year" || r.Groups[0].Rules[0].Op != "gte" {
		t.Fatalf("unexpected: %+v", r.Groups[0].Rules[0])
	}
}

func TestParseRulesEmptyDefaultsToMatchAll(t *testing.T) {
	r, err := routing.ParseRules([]byte(`{}`))
	if err != nil { t.Fatal(err) }
	if r.Match != "all" || len(r.Groups) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestParseRulesNilOrEmptyBytes(t *testing.T) {
	for _, raw := range [][]byte{nil, {}} {
		r, err := routing.ParseRules(raw)
		if err != nil { t.Fatalf("ParseRules(%q): %v", raw, err) }
		if r.Match != "all" || len(r.Groups) != 0 {
			t.Fatalf("unexpected: %+v", r)
		}
	}
}

func TestParseRulesGroupMatchDefaultsToAll(t *testing.T) {
	// A group with omitted match should default to "all" after parse.
	raw := []byte(`{"match":"all","groups":[{"rules":[{"field":"year","op":"eq","value":2000}]}]}`)
	r, err := routing.ParseRules(raw)
	if err != nil { t.Fatal(err) }
	if r.Groups[0].Match != "all" {
		t.Fatalf("group match: got %q want all", r.Groups[0].Match)
	}
}

func TestParseRulesMalformedJSONReturnsError(t *testing.T) {
	if _, err := routing.ParseRules([]byte(`not json`)); err == nil {
		t.Fatal("expected error from malformed json")
	}
}

func TestValidateRulesAcceptsEmptyGroups(t *testing.T) {
	// catch-all: empty groups must be valid.
	if err := routing.ValidateRules(routing.Rules{Match: "all"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRulesRejectsBadCombinator(t *testing.T) {
	bad := routing.Rules{Match: "either", Groups: []routing.Group{}}
	if err := routing.ValidateRules(bad); err == nil {
		t.Fatal("expected error from bad top-level match")
	}
	bad2 := routing.Rules{
		Match:  "all",
		Groups: []routing.Group{{Match: "either", Rules: nil}},
	}
	if err := routing.ValidateRules(bad2); err == nil {
		t.Fatal("expected error from bad group match")
	}
}

func TestValidateRulesRejectsUnknownField(t *testing.T) {
	r := routing.Rules{
		Match: "all",
		Groups: []routing.Group{{
			Match: "all",
			Rules: []routing.Rule{{Field: "banana", Op: "eq", Value: json.RawMessage(`"x"`)}},
		}},
	}
	if err := routing.ValidateRules(r); err == nil {
		t.Fatal("expected error from unknown field")
	}
}

func TestValidateRulesRejectsUnknownOp(t *testing.T) {
	r := routing.Rules{
		Match: "all",
		Groups: []routing.Group{{
			Match: "all",
			Rules: []routing.Rule{{Field: "year", Op: "smaller_than_or_equal_or_neighbour", Value: json.RawMessage(`5`)}},
		}},
	}
	if err := routing.ValidateRules(r); err == nil {
		t.Fatal("expected error from unknown op")
	}
}
