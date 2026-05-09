package routing

import (
	"encoding/json"
	"regexp"
	"strings"
)

// opSet is the authoritative set of supported operator names.
var opSet = map[string]struct{}{
	"eq":          {},
	"ne":          {},
	"in":          {},
	"not_in":      {},
	"gt":          {},
	"gte":         {},
	"lt":          {},
	"lte":         {},
	"between":     {},
	"contains":    {},
	"starts_with": {},
	"regex":       {},
}

// knownOpImpl is the real implementation used by both the exported KnownOp
// function and the package-var KnownOp hook wired up in rules.go.
func knownOpImpl(op string) bool {
	_, ok := opSet[op]
	return ok
}

// Apply runs the named operator against actual and the JSON-encoded raw value.
// Returns (matched, traceNote). traceNote is non-empty only when the rule fell
// back to false because of a type problem or other input issue — the evaluator
// surfaces it in match_trace.
func Apply(op string, actual any, raw json.RawMessage) (bool, string) {
	if actual == nil {
		return false, "missing actual"
	}
	switch op {
	case "eq":
		return cmpEq(actual, raw)
	case "ne":
		matched, note := cmpEq(actual, raw)
		if note != "" {
			return false, note
		}
		return !matched, ""
	case "in":
		return cmpIn(actual, raw)
	case "not_in":
		matched, note := cmpIn(actual, raw)
		if note != "" {
			return false, note
		}
		return !matched, ""
	case "gt", "gte", "lt", "lte":
		return cmpNumericOrder(op, actual, raw)
	case "between":
		return cmpBetween(actual, raw)
	case "contains":
		return cmpContains(actual, raw)
	case "starts_with":
		return cmpStartsWith(actual, raw)
	case "regex":
		return cmpRegex(actual, raw)
	}
	return false, "unknown op: " + op
}

// coerceNumber converts numeric types (including json.Number) to float64.
// Returns (0, false) for non-numeric types.
func coerceNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// cmpEq implements eq semantics:
//   - string actual vs JSON string: case-insensitive comparison
//   - numeric actual vs JSON number: coerce both to float64
//   - bool actual vs JSON bool: direct equality
//   - mismatched kinds → (false, "type mismatch")
//
// Note: "5" == 5 is a type mismatch (string vs number).
func cmpEq(actual any, raw json.RawMessage) (bool, string) {
	switch a := actual.(type) {
	case string:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return false, "type mismatch"
		}
		return strings.EqualFold(a, s), ""
	case bool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return false, "type mismatch"
		}
		return a == b, ""
	default:
		af, aOk := coerceNumber(actual)
		if !aOk {
			return false, "type mismatch"
		}
		// Unmarshal value as json.Number to preserve precision
		var jn json.Number
		if err := json.Unmarshal(raw, &jn); err != nil {
			return false, "type mismatch"
		}
		vf, err := jn.Float64()
		if err != nil {
			return false, "type mismatch"
		}
		return af == vf, ""
	}
}

// cmpIn checks whether a scalar actual appears in a JSON array value.
// Case-insensitive for string comparisons. Returns "type mismatch" if actual
// is not a scalar type that cmpEq supports.
func cmpIn(actual any, raw json.RawMessage) (bool, string) {
	// Verify actual is a supported scalar type
	switch actual.(type) {
	case string, bool:
		// ok
	default:
		if _, ok := coerceNumber(actual); !ok {
			return false, "type mismatch"
		}
	}

	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return false, "type mismatch"
	}
	for _, elem := range elements {
		matched, _ := cmpEq(actual, elem)
		if matched {
			return true, ""
		}
	}
	return false, ""
}

// cmpNumericOrder implements gt, gte, lt, lte comparisons.
// Both actual and the JSON value must be numeric; mismatches → "type mismatch".
func cmpNumericOrder(op string, actual any, raw json.RawMessage) (bool, string) {
	af, ok := coerceNumber(actual)
	if !ok {
		return false, "type mismatch"
	}
	var jn json.Number
	if err := json.Unmarshal(raw, &jn); err != nil {
		return false, "type mismatch"
	}
	vf, err := jn.Float64()
	if err != nil {
		return false, "type mismatch"
	}
	switch op {
	case "gt":
		return af > vf, ""
	case "gte":
		return af >= vf, ""
	case "lt":
		return af < vf, ""
	case "lte":
		return af <= vf, ""
	}
	return false, "unknown op: " + op
}

// cmpBetween implements inclusive range check: value must be [low, high].
// Both actual and both bounds must be numeric.
func cmpBetween(actual any, raw json.RawMessage) (bool, string) {
	af, ok := coerceNumber(actual)
	if !ok {
		return false, "type mismatch"
	}
	var nums []json.Number
	if err := json.Unmarshal(raw, &nums); err != nil || len(nums) != 2 {
		return false, "type mismatch"
	}
	low, err := nums[0].Float64()
	if err != nil {
		return false, "type mismatch"
	}
	high, err := nums[1].Float64()
	if err != nil {
		return false, "type mismatch"
	}
	return af >= low && af <= high, ""
}

// cmpContains handles two modes:
//   - string actual + JSON string value: substring check, case-insensitive
//   - []string actual + JSON string value: membership check, case-insensitive
//
// Any other combination → "type mismatch".
func cmpContains(actual any, raw json.RawMessage) (bool, string) {
	var needle string
	if err := json.Unmarshal(raw, &needle); err != nil {
		return false, "type mismatch"
	}
	needleLower := strings.ToLower(needle)

	switch a := actual.(type) {
	case string:
		return strings.Contains(strings.ToLower(a), needleLower), ""
	case []string:
		for _, elem := range a {
			if strings.EqualFold(elem, needle) {
				return true, ""
			}
		}
		return false, ""
	default:
		return false, "type mismatch"
	}
}

// cmpStartsWith checks whether a string actual begins with a JSON string value,
// case-insensitive.
func cmpStartsWith(actual any, raw json.RawMessage) (bool, string) {
	a, ok := actual.(string)
	if !ok {
		return false, "type mismatch"
	}
	var prefix string
	if err := json.Unmarshal(raw, &prefix); err != nil {
		return false, "type mismatch"
	}
	return strings.HasPrefix(strings.ToLower(a), strings.ToLower(prefix)), ""
}

// cmpRegex matches a string actual against a RE2 regex encoded in raw.
// Invalid regex patterns return (false, "invalid regex: <err>") — never panic.
func cmpRegex(actual any, raw json.RawMessage) (bool, string) {
	a, ok := actual.(string)
	if !ok {
		return false, "type mismatch"
	}
	var pattern string
	if err := json.Unmarshal(raw, &pattern); err != nil {
		return false, "type mismatch"
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, "invalid regex: " + err.Error()
	}
	return re.MatchString(a), ""
}
