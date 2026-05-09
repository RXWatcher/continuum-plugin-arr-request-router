package routing

// RuleResult captures the outcome of evaluating a single rule predicate.
type RuleResult struct {
	Field   string `json:"field"`
	Op      string `json:"op"`
	Matched bool   `json:"matched"`
	Note    string `json:"note,omitempty"` // type mismatch / invalid regex / missing field
}

// GroupResult captures the outcome of evaluating one Group.
type GroupResult struct {
	Match   string       `json:"match"`
	Matched bool         `json:"matched"`
	Rules   []RuleResult `json:"rules"`
}

// ArrTrace is the per-candidate evaluation record produced for one registered
// *arr during a routing pass.
type ArrTrace struct {
	ArrID   int64         `json:"arr_id"`
	ArrName string        `json:"arr_name"`
	Match   string        `json:"match"`
	Matched bool          `json:"matched"`
	Groups  []GroupResult `json:"groups"`
}

// Trace is the top-level diagnostic record returned by the router and
// surfaced by the route-test admin endpoint (Task 9.4).
type Trace struct {
	TMDBPrimaryError string     `json:"tmdb_primary_error,omitempty"`
	KeywordsError    string     `json:"keywords_error,omitempty"`
	ContentRatingErr string     `json:"content_rating_error,omitempty"`
	Candidates       []ArrTrace `json:"candidates"`
	ChosenArrID      *int64     `json:"chosen_arr_id,omitempty"`
}
