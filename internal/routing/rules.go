package routing

import (
	"encoding/json"
	"fmt"
)

// Combinator: "all" or "any".
type Combinator string

const (
	MatchAll Combinator = "all"
	MatchAny Combinator = "any"
)

// Rules is the rule_json structure stored per registered_arr.
type Rules struct {
	Match  Combinator `json:"match"`
	Groups []Group    `json:"groups"`
}

// Group combines a list of Rules with its own match combinator.
type Group struct {
	Match Combinator `json:"match"`
	Rules []Rule     `json:"rules"`
}

// Rule is a single (field, op, value) predicate.
type Rule struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// ParseRules tolerantly parses a JSON blob from registered_arr.rules_json.
// Empty input or an empty object both produce a permissive default
// (Match=all, Groups=nil) which matches every event — useful for the
// "catch-all" *arr at the lowest priority.
func ParseRules(raw []byte) (Rules, error) {
	if len(raw) == 0 {
		return Rules{Match: MatchAll}, nil
	}
	var r Rules
	if err := json.Unmarshal(raw, &r); err != nil {
		return Rules{}, fmt.Errorf("parse rules: %w", err)
	}
	if r.Match == "" {
		r.Match = MatchAll
	}
	for i := range r.Groups {
		if r.Groups[i].Match == "" {
			r.Groups[i].Match = MatchAll
		}
	}
	return r, nil
}

// KnownField and KnownOp are validator hooks. Tasks 3.2 (operators) and
// 3.3-3.5 (field accessors) replace these with real registries. The
// placeholder values here only cover the two names referenced by this
// task's own tests — they will be overwritten before any real call site
// uses them.
var (
	KnownField = func(name string) bool {
		switch name {
		case "year":
			return true
		}
		return false
	}
	KnownOp = func(op string) bool {
		switch op {
		case "eq", "gte":
			return true
		}
		return false
	}
)

// ValidateRules walks the structure and returns an error on the first
// problem encountered (unknown field, unknown op, bad combinator). An
// empty Groups list is valid (matches everything).
func ValidateRules(r Rules) error {
	if r.Match != MatchAll && r.Match != MatchAny {
		return fmt.Errorf("invalid top-level match: %q", r.Match)
	}
	for gi, g := range r.Groups {
		if g.Match != MatchAll && g.Match != MatchAny {
			return fmt.Errorf("group %d: invalid match: %q", gi, g.Match)
		}
		for ri, ru := range g.Rules {
			if !KnownField(ru.Field) {
				return fmt.Errorf("group %d rule %d: unknown field %q", gi, ri, ru.Field)
			}
			if !KnownOp(ru.Op) {
				return fmt.Errorf("group %d rule %d: unknown op %q", gi, ri, ru.Op)
			}
		}
	}
	return nil
}
