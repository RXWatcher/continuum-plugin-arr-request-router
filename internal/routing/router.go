package routing

import "context"

// Enricher fetches TMDB-derived field data on demand. The router only
// invokes the methods whose data is actually referenced by candidate rules.
type Enricher interface {
	Primary(ctx context.Context, mediaType string, tmdbID int) (*TMDBPrimary, error)
	Keywords(ctx context.Context, mediaType string, tmdbID int) ([]string, error)
	ContentRating(ctx context.Context, mediaType string, tmdbID int) (string, error)
}

// Candidate is a registered *arr in the routing pool. Caller is responsible
// for sorting candidates by (priority ASC, id ASC) before calling Decide.
type Candidate struct {
	ID    int64
	Name  string
	Kind  string // "radarr" | "sonarr"
	Rules Rules
}

// Decide picks the first candidate whose rules match. Returns:
//   - chosen: the chosen candidate's ID, or nil if no candidate matched.
//   - trace: the full diagnostic trace, with per-candidate ArrTrace entries
//     and any TMDB enrichment errors.
//
// Pre-conditions: candidates must already be sorted by (priority ASC, id ASC).
// Decide filters by kind internally (movie → radarr, tv → sonarr).
func Decide(ctx context.Context, candidates []Candidate, ev RequestEvent, enr Enricher) (*int64, Trace) {
	var trace Trace

	relevant := filterByKind(candidates, ev.MediaType)
	if len(relevant) == 0 {
		return nil, trace
	}

	needPrimary, needKeywords, needRating := analyzeNeeds(relevant)

	rctx := Context{Event: ev}

	if needPrimary {
		p, err := enr.Primary(ctx, ev.MediaType, ev.TMDBID)
		if err != nil {
			trace.TMDBPrimaryError = err.Error()
		} else {
			rctx.Primary = p
		}
	}
	if needKeywords {
		k, err := enr.Keywords(ctx, ev.MediaType, ev.TMDBID)
		if err != nil {
			trace.KeywordsError = err.Error()
		} else {
			rctx.Keywords = k
		}
	}
	if needRating {
		r, err := enr.ContentRating(ctx, ev.MediaType, ev.TMDBID)
		if err != nil {
			trace.ContentRatingErr = err.Error()
		} else {
			rctx.ContentRating = r
		}
	}

	trace.Candidates = make([]ArrTrace, 0, len(relevant))
	for _, c := range relevant {
		matched, groups := Evaluate(c.Rules, rctx)
		trace.Candidates = append(trace.Candidates, ArrTrace{
			ArrID:   c.ID,
			ArrName: c.Name,
			Match:   string(c.Rules.Match),
			Matched: matched,
			Groups:  groups,
		})
		if matched {
			id := c.ID
			trace.ChosenArrID = &id
			return &id, trace
		}
	}

	return nil, trace
}

// filterByKind returns only the candidates matching the kind implied by
// mediaType. Returns nil (not an empty slice) for unknown media types so the
// caller can detect the "unknown" case cleanly.
func filterByKind(in []Candidate, mediaType string) []Candidate {
	var kind string
	switch mediaType {
	case "movie":
		kind = "radarr"
	case "tv":
		kind = "sonarr"
	default:
		return nil
	}
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// analyzeNeeds inspects every rule in every candidate to determine which
// enricher methods are required. This keeps enrichment lazy: we only call
// the methods that could actually affect the routing outcome.
func analyzeNeeds(cs []Candidate) (primary, keywords, rating bool) {
	for _, c := range cs {
		for _, g := range c.Rules.Groups {
			for _, r := range g.Rules {
				grp, ok := FieldGroupOf(r.Field)
				if !ok {
					continue
				}
				switch grp {
				case GroupB:
					primary = true
				case GroupCKeywords:
					keywords = true
				case GroupCContentRating:
					rating = true
				}
			}
		}
	}
	return
}
