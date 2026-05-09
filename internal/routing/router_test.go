package routing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// fakeEnricher records call counts and returns canned values/errors.
type fakeEnricher struct {
	primaryCalls  int
	keywordsCalls int
	ratingCalls   int
	primary       *TMDBPrimary
	keywords      []string
	rating        string
	primaryErr    error
	keywordsErr   error
	ratingErr     error
}

func (f *fakeEnricher) Primary(ctx context.Context, mt string, id int) (*TMDBPrimary, error) {
	f.primaryCalls++
	return f.primary, f.primaryErr
}
func (f *fakeEnricher) Keywords(ctx context.Context, mt string, id int) ([]string, error) {
	f.keywordsCalls++
	return f.keywords, f.keywordsErr
}
func (f *fakeEnricher) ContentRating(ctx context.Context, mt string, id int) (string, error) {
	f.ratingCalls++
	return f.rating, f.ratingErr
}

// helpers to build Rules with a single group and single rule
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func singleRuleRules(field, op string, value any) Rules {
	return Rules{
		Match: MatchAll,
		Groups: []Group{
			{
				Match: MatchAll,
				Rules: []Rule{
					{Field: field, Op: op, Value: mustJSON(value)},
				},
			},
		},
	}
}

func catchAllRules() Rules {
	return Rules{Match: MatchAll, Groups: nil}
}

// TestDecideFirstMatchWins: two radarr candidates; first doesn't match (year<1990),
// second matches (year>=2000). Movie event year=2003.
func TestDecideFirstMatchWins(t *testing.T) {
	c1 := Candidate{
		ID:    1,
		Name:  "Radarr-Old",
		Kind:  "radarr",
		Rules: singleRuleRules("year", "lt", 1990),
	}
	c2 := Candidate{
		ID:    2,
		Name:  "Radarr-New",
		Kind:  "radarr",
		Rules: singleRuleRules("year", "gte", 2000),
	}
	ev := RequestEvent{MediaType: "movie", Year: 2003, TMDBID: 42}
	enr := &fakeEnricher{}

	chosen, trace := Decide(context.Background(), []Candidate{c1, c2}, ev, enr)

	if chosen == nil {
		t.Fatal("expected a match, got nil")
	}
	if *chosen != 2 {
		t.Errorf("expected chosen=2, got %d", *chosen)
	}
	if len(trace.Candidates) != 2 {
		t.Fatalf("expected 2 candidates in trace, got %d", len(trace.Candidates))
	}
	if trace.Candidates[0].Matched {
		t.Error("first candidate should not have matched")
	}
	if !trace.Candidates[1].Matched {
		t.Error("second candidate should have matched")
	}
	if trace.ChosenArrID == nil || *trace.ChosenArrID != 2 {
		t.Error("ChosenArrID should be 2")
	}
}

// TestDecideKindFilterMovieOnlyRadarr: 3 candidates (1 sonarr, 2 radarr); movie event.
// Only the 2 radarr candidates appear in trace.
func TestDecideKindFilterMovieOnlyRadarr(t *testing.T) {
	candidates := []Candidate{
		{ID: 10, Name: "Sonarr-1", Kind: "sonarr", Rules: catchAllRules()},
		{ID: 20, Name: "Radarr-1", Kind: "radarr", Rules: singleRuleRules("year", "lt", 1990)},
		{ID: 30, Name: "Radarr-2", Kind: "radarr", Rules: catchAllRules()},
	}
	ev := RequestEvent{MediaType: "movie", Year: 2003}
	enr := &fakeEnricher{}

	_, trace := Decide(context.Background(), candidates, ev, enr)

	if len(trace.Candidates) != 2 {
		t.Fatalf("expected 2 candidates in trace, got %d", len(trace.Candidates))
	}
	for _, c := range trace.Candidates {
		if c.ArrID == 10 {
			t.Error("sonarr candidate should not appear in trace for movie event")
		}
	}
}

// TestDecideKindFilterTVOnlySonarr: 3 candidates (1 radarr, 2 sonarr); tv event.
func TestDecideKindFilterTVOnlySonarr(t *testing.T) {
	candidates := []Candidate{
		{ID: 10, Name: "Radarr-1", Kind: "radarr", Rules: catchAllRules()},
		{ID: 20, Name: "Sonarr-1", Kind: "sonarr", Rules: singleRuleRules("year", "lt", 1990)},
		{ID: 30, Name: "Sonarr-2", Kind: "sonarr", Rules: catchAllRules()},
	}
	ev := RequestEvent{MediaType: "tv", Year: 2003}
	enr := &fakeEnricher{}

	_, trace := Decide(context.Background(), candidates, ev, enr)

	if len(trace.Candidates) != 2 {
		t.Fatalf("expected 2 candidates in trace, got %d", len(trace.Candidates))
	}
	for _, c := range trace.Candidates {
		if c.ArrID == 10 {
			t.Error("radarr candidate should not appear in trace for tv event")
		}
	}
}

// TestDecideUnknownMediaType: ev.MediaType="podcast" → (nil, Trace{}) with no candidates.
func TestDecideUnknownMediaType(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: catchAllRules()},
	}
	ev := RequestEvent{MediaType: "podcast"}
	enr := &fakeEnricher{}

	chosen, trace := Decide(context.Background(), candidates, ev, enr)

	if chosen != nil {
		t.Error("expected nil chosen for unknown media type")
	}
	if len(trace.Candidates) != 0 {
		t.Errorf("expected no candidates in trace, got %d", len(trace.Candidates))
	}
	if enr.primaryCalls+enr.keywordsCalls+enr.ratingCalls != 0 {
		t.Error("enricher should not be called for unknown media type")
	}
}

// TestDecideEmptyCandidatesReturnsNoMatch: 0 candidates → chosen=nil, no enricher calls.
func TestDecideEmptyCandidatesReturnsNoMatch(t *testing.T) {
	ev := RequestEvent{MediaType: "movie", TMDBID: 1}
	enr := &fakeEnricher{}

	chosen, trace := Decide(context.Background(), []Candidate{}, ev, enr)

	if chosen != nil {
		t.Error("expected nil chosen for empty candidates")
	}
	if len(trace.Candidates) != 0 {
		t.Error("expected no candidates in trace")
	}
	if enr.primaryCalls+enr.keywordsCalls+enr.ratingCalls != 0 {
		t.Error("enricher should not be called for empty candidates")
	}
}

// TestDecideAllRulesGroupASkipsEnricher: candidate uses only "year" and "mediaType".
// All enricher call counts should be 0.
func TestDecideAllRulesGroupASkipsEnricher(t *testing.T) {
	rules := Rules{
		Match: MatchAll,
		Groups: []Group{
			{
				Match: MatchAll,
				Rules: []Rule{
					{Field: "year", Op: "gte", Value: mustJSON(2000)},
					{Field: "mediaType", Op: "eq", Value: mustJSON("movie")},
				},
			},
		},
	}
	candidates := []Candidate{{ID: 1, Kind: "radarr", Rules: rules}}
	ev := RequestEvent{MediaType: "movie", Year: 2005, TMDBID: 99}
	enr := &fakeEnricher{}

	Decide(context.Background(), candidates, ev, enr)

	if enr.primaryCalls != 0 {
		t.Errorf("expected 0 Primary calls, got %d", enr.primaryCalls)
	}
	if enr.keywordsCalls != 0 {
		t.Errorf("expected 0 Keywords calls, got %d", enr.keywordsCalls)
	}
	if enr.ratingCalls != 0 {
		t.Errorf("expected 0 ContentRating calls, got %d", enr.ratingCalls)
	}
}

// TestDecideGroupBFieldTriggersPrimary: candidate uses "original_language".
// Primary called once; Keywords/Rating not called.
func TestDecideGroupBFieldTriggersPrimary(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: singleRuleRules("original_language", "eq", "en")},
	}
	ev := RequestEvent{MediaType: "movie", TMDBID: 5}
	enr := &fakeEnricher{
		primary: &TMDBPrimary{OriginalLanguage: "en"},
	}

	Decide(context.Background(), candidates, ev, enr)

	if enr.primaryCalls != 1 {
		t.Errorf("expected 1 Primary call, got %d", enr.primaryCalls)
	}
	if enr.keywordsCalls != 0 {
		t.Errorf("expected 0 Keywords calls, got %d", enr.keywordsCalls)
	}
	if enr.ratingCalls != 0 {
		t.Errorf("expected 0 ContentRating calls, got %d", enr.ratingCalls)
	}
}

// TestDecideKeywordsFieldTriggersOnlyKeywords: candidate uses "keywords".
// Keywords called once; Primary NOT called (no Group B field).
func TestDecideKeywordsFieldTriggersOnlyKeywords(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: singleRuleRules("keywords", "contains", "action")},
	}
	ev := RequestEvent{MediaType: "movie", TMDBID: 5}
	enr := &fakeEnricher{keywords: []string{"action", "thriller"}}

	Decide(context.Background(), candidates, ev, enr)

	if enr.keywordsCalls != 1 {
		t.Errorf("expected 1 Keywords call, got %d", enr.keywordsCalls)
	}
	if enr.primaryCalls != 0 {
		t.Errorf("expected 0 Primary calls, got %d", enr.primaryCalls)
	}
}

// TestDecideContentRatingFieldTriggersRatingOnly: candidate uses "content_rating".
// ContentRating called once; Primary/Keywords not called.
func TestDecideContentRatingFieldTriggersRatingOnly(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: singleRuleRules("content_rating", "eq", "R")},
	}
	ev := RequestEvent{MediaType: "movie", TMDBID: 5}
	enr := &fakeEnricher{rating: "R"}

	Decide(context.Background(), candidates, ev, enr)

	if enr.ratingCalls != 1 {
		t.Errorf("expected 1 ContentRating call, got %d", enr.ratingCalls)
	}
	if enr.primaryCalls != 0 {
		t.Errorf("expected 0 Primary calls, got %d", enr.primaryCalls)
	}
	if enr.keywordsCalls != 0 {
		t.Errorf("expected 0 Keywords calls, got %d", enr.keywordsCalls)
	}
}

// TestDecideTMDBPrimaryFailureRecordedInTraceAndRoutingContinues: candidate uses
// "original_language". Enricher.Primary returns an error. Trace records error;
// rule evaluates false (missing); chosen is nil.
func TestDecideTMDBPrimaryFailureRecordedInTraceAndRoutingContinues(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: singleRuleRules("original_language", "eq", "en")},
	}
	ev := RequestEvent{MediaType: "movie", TMDBID: 5}
	enr := &fakeEnricher{primaryErr: errors.New("tmdb unavailable")}

	chosen, trace := Decide(context.Background(), candidates, ev, enr)

	if trace.TMDBPrimaryError == "" {
		t.Error("expected TMDBPrimaryError to be set")
	}
	if chosen != nil {
		t.Error("expected nil chosen when enrichment failed and rule can't match")
	}
	if len(trace.Candidates) != 1 {
		t.Fatalf("expected 1 candidate in trace, got %d", len(trace.Candidates))
	}
	if trace.Candidates[0].Matched {
		t.Error("candidate should not have matched when primary data is missing")
	}
}

// TestDecideKeywordsFailureRecorded: same shape for keywords error.
func TestDecideKeywordsFailureRecorded(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: singleRuleRules("keywords", "contains", "action")},
	}
	ev := RequestEvent{MediaType: "movie", TMDBID: 5}
	enr := &fakeEnricher{keywordsErr: errors.New("keywords fetch failed")}

	chosen, trace := Decide(context.Background(), candidates, ev, enr)

	if trace.KeywordsError == "" {
		t.Error("expected KeywordsError to be set")
	}
	if chosen != nil {
		t.Error("expected nil chosen when keywords fetch failed")
	}
	if len(trace.Candidates) != 1 {
		t.Fatalf("expected 1 candidate in trace, got %d", len(trace.Candidates))
	}
	if trace.Candidates[0].Matched {
		t.Error("candidate should not have matched when keywords data is missing")
	}
}

// TestDecideContentRatingFailureRecorded: same shape for content_rating error.
func TestDecideContentRatingFailureRecorded(t *testing.T) {
	candidates := []Candidate{
		{ID: 1, Kind: "radarr", Rules: singleRuleRules("content_rating", "eq", "R")},
	}
	ev := RequestEvent{MediaType: "movie", TMDBID: 5}
	enr := &fakeEnricher{ratingErr: errors.New("content rating fetch failed")}

	chosen, trace := Decide(context.Background(), candidates, ev, enr)

	if trace.ContentRatingErr == "" {
		t.Error("expected ContentRatingErr to be set")
	}
	if chosen != nil {
		t.Error("expected nil chosen when content rating fetch failed")
	}
	if len(trace.Candidates) != 1 {
		t.Fatalf("expected 1 candidate in trace, got %d", len(trace.Candidates))
	}
	if trace.Candidates[0].Matched {
		t.Error("candidate should not have matched when content rating data is missing")
	}
}

// TestDecideNoMatchReturnsNilChosenTracePopulated: 2 candidates, neither matches.
// chosen=nil, ChosenArrID=nil, len(trace.Candidates)==2, both Matched=false.
func TestDecideNoMatchReturnsNilChosenTracePopulated(t *testing.T) {
	c1 := Candidate{ID: 1, Name: "A", Kind: "radarr", Rules: singleRuleRules("year", "lt", 1900)}
	c2 := Candidate{ID: 2, Name: "B", Kind: "radarr", Rules: singleRuleRules("year", "lt", 1900)}
	ev := RequestEvent{MediaType: "movie", Year: 2000}
	enr := &fakeEnricher{}

	chosen, trace := Decide(context.Background(), []Candidate{c1, c2}, ev, enr)

	if chosen != nil {
		t.Error("expected nil chosen")
	}
	if trace.ChosenArrID != nil {
		t.Error("expected nil ChosenArrID")
	}
	if len(trace.Candidates) != 2 {
		t.Fatalf("expected 2 candidates in trace, got %d", len(trace.Candidates))
	}
	for i, c := range trace.Candidates {
		if c.Matched {
			t.Errorf("candidate %d should not have matched", i)
		}
	}
}

// TestDecideChosenArrIDSetWhenMatch: 1 candidate that matches.
// trace.ChosenArrID equals the candidate's id.
func TestDecideChosenArrIDSetWhenMatch(t *testing.T) {
	candidates := []Candidate{
		{ID: 77, Name: "Radarr-Main", Kind: "radarr", Rules: singleRuleRules("year", "gte", 2000)},
	}
	ev := RequestEvent{MediaType: "movie", Year: 2010}
	enr := &fakeEnricher{}

	chosen, trace := Decide(context.Background(), candidates, ev, enr)

	if chosen == nil {
		t.Fatal("expected a match")
	}
	if *chosen != 77 {
		t.Errorf("expected chosen=77, got %d", *chosen)
	}
	if trace.ChosenArrID == nil || *trace.ChosenArrID != 77 {
		t.Error("trace.ChosenArrID should be 77")
	}
}

// TestDecideCatchallEmptyRulesAlwaysMatchesFirst: 2 candidates; first doesn't match,
// second has empty Rules (catch-all). Confirm second is chosen.
func TestDecideCatchallEmptyRulesAlwaysMatchesFirst(t *testing.T) {
	c1 := Candidate{ID: 1, Name: "Strict", Kind: "radarr", Rules: singleRuleRules("year", "lt", 1900)}
	c2 := Candidate{ID: 2, Name: "Catchall", Kind: "radarr", Rules: catchAllRules()}
	ev := RequestEvent{MediaType: "movie", Year: 2005}
	enr := &fakeEnricher{}

	chosen, trace := Decide(context.Background(), []Candidate{c1, c2}, ev, enr)

	if chosen == nil {
		t.Fatal("expected catch-all to match")
	}
	if *chosen != 2 {
		t.Errorf("expected chosen=2 (catch-all), got %d", *chosen)
	}
	if len(trace.Candidates) != 2 {
		t.Fatalf("expected 2 candidates in trace, got %d", len(trace.Candidates))
	}
	if trace.Candidates[0].Matched {
		t.Error("first candidate should not have matched")
	}
	if !trace.Candidates[1].Matched {
		t.Error("catch-all candidate should have matched")
	}
}
