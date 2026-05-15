package routing_test

import (
	"encoding/json"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
)

func TestApply(t *testing.T) {
	cases := []struct {
		name   string
		op     string
		actual any
		value  string
		want   bool
	}{
		// eq / ne — case-insensitive on strings
		{"eq string ci", "eq", "Animation", `"animation"`, true},
		{"eq string mismatch", "eq", "Animation", `"comedy"`, false},
		{"ne true", "ne", "Animation", `"comedy"`, true},
		{"ne false", "ne", "Animation", `"animation"`, false},
		{"eq int", "eq", 2003, `2003`, true},
		{"eq float", "eq", 8.5, `8.5`, true},
		{"eq bool", "eq", true, `true`, true},

		// in / not_in
		{"in match", "in", "ja", `["en","ja","ko"]`, true},
		{"in miss", "in", "fr", `["en","ja","ko"]`, false},
		{"not_in match", "not_in", "fr", `["en","ja","ko"]`, true},
		{"not_in miss", "not_in", "ja", `["en","ja","ko"]`, false},
		{"in numeric", "in", 5, `[1,3,5,7]`, true},
		{"in case-insens", "in", "JA", `["en","ja","ko"]`, true},

		// numeric comparisons
		{"gt int", "gt", 2005, `2000`, true},
		{"gt fail", "gt", 1999, `2000`, false},
		{"gte equal", "gte", 2000, `2000`, true},
		{"lt float", "lt", 7.4, `8.0`, true},
		{"lte equal", "lte", 8.0, `8.0`, true},
		{"between in", "between", 2005, `[2000,2010]`, true},
		{"between low", "between", 2000, `[2000,2010]`, true},
		{"between high", "between", 2010, `[2000,2010]`, true},
		{"between out", "between", 1999, `[2000,2010]`, false},
		{"between three elem", "between", 5, `[1,10,99]`, false},
		{"between one elem", "between", 5, `[5]`, false},

		// contains — substring on string, membership on string-array
		{"contains substr", "contains", "Sci-Fi & Fantasy", `"fantasy"`, true},
		{"contains substr ci", "contains", "Sci-Fi & Fantasy", `"FANTASY"`, true},
		{"contains substr miss", "contains", "Drama", `"comedy"`, false},
		{"contains array hit", "contains", []string{"Animation", "Action"}, `"animation"`, true},
		{"contains array hit ci", "contains", []string{"Animation", "Action"}, `"ACTION"`, true},
		{"contains array miss", "contains", []string{"Drama"}, `"action"`, false},

		// starts_with
		{"starts_with", "starts_with", "The Matrix", `"the "`, true},
		{"starts_with ci", "starts_with", "The Matrix", `"THE "`, true},
		{"starts_with miss", "starts_with", "The Matrix", `"matrix"`, false},

		// regex (RE2)
		{"regex match", "regex", "abc-123", `"^abc-\\d+$"`, true},
		{"regex no-match", "regex", "abc-XYZ", `"^abc-\\d+$"`, false},
		{"regex invalid", "regex", "abc", `"["`, false},

		// type-mismatch fall-throughs
		{"gt vs string", "gt", "string", `5`, false},
		{"between vs str", "between", "x", `[1,2]`, false},
		{"in vs object", "in", map[string]any{}, `["a"]`, false},
		{"missing actual", "eq", nil, `"x"`, false},
		{"contains numeric actual", "contains", 42, `"4"`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := routing.Apply(c.op, c.actual, json.RawMessage(c.value))
			if got != c.want {
				t.Fatalf("Apply(%q, %v, %s) = %v, want %v", c.op, c.actual, c.value, got, c.want)
			}
		})
	}
}

func TestApplyEmitsTraceNoteOnFailureModes(t *testing.T) {
	// type mismatch → note "type mismatch"
	if _, note := routing.Apply("gt", "x", json.RawMessage(`5`)); note == "" {
		t.Error("expected traceNote on type mismatch")
	}
	// invalid regex → note containing "invalid regex"
	if _, note := routing.Apply("regex", "abc", json.RawMessage(`"["`)); note == "" {
		t.Error("expected traceNote on invalid regex")
	}
	// happy path → empty traceNote
	if _, note := routing.Apply("eq", "a", json.RawMessage(`"a"`)); note != "" {
		t.Errorf("unexpected traceNote on success: %q", note)
	}
}

func TestApplyUnknownOpReturnsFalseWithNote(t *testing.T) {
	matched, note := routing.Apply("bogus", "x", json.RawMessage(`"x"`))
	if matched {
		t.Error("unknown op must be false")
	}
	if note == "" {
		t.Error("unknown op must emit traceNote")
	}
}

func TestKnownOp(t *testing.T) {
	for _, op := range []string{"eq", "ne", "in", "not_in", "gt", "gte", "lt", "lte", "between", "contains", "starts_with", "regex"} {
		if !routing.KnownOp(op) {
			t.Errorf("KnownOp(%q) = false", op)
		}
	}
	if routing.KnownOp("bogus") {
		t.Error("KnownOp(bogus) = true")
	}
}

// Ensure the rules-validator hook now uses the real KnownOp.
func TestValidateRulesUsesRealKnownOp(t *testing.T) {
	r := routing.Rules{
		Match: routing.MatchAll,
		Groups: []routing.Group{{
			Match: routing.MatchAll,
			Rules: []routing.Rule{{Field: "year", Op: "between", Value: json.RawMessage(`[2000,2010]`)}},
		}},
	}
	// `between` was NOT in the placeholder KnownOp from Task 3.1 (only eq/gte
	// were). Now that 3.2 has wired the real list, `between` should be accepted.
	if err := routing.ValidateRules(r); err != nil {
		t.Fatalf("ValidateRules: %v", err)
	}
}
