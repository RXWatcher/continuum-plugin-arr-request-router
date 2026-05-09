package routing

// RequestEvent is the subset of the continuum.requests event payload we care about.
type RequestEvent struct {
	RequestID        string
	MediaType        string // "movie" | "tv"
	LibraryID        string
	Year             int
	RequesterUserID  string
	RequesterIsAdmin bool
	Title            string
	TMDBID           int
	PosterURL        string
}

// Context is what GetField queries. The lazily-fetched groups
// (Primary, Keywords, ContentRating) start zero-valued and get populated by
// the router only when a rule references them.
type Context struct {
	Event         RequestEvent
	Primary       *TMDBPrimary
	Keywords      []string
	ContentRating string
}

// TMDBPrimary holds the fields produced by /movie/{id} or /tv/{id}.
type TMDBPrimary struct {
	MediaType string // "movie" | "tv" — redundant with Event but lets fields.go decide kind-only fields without consulting Event

	// Common
	OriginalLanguage    string
	OriginalTitle       string
	Genres              []string
	Runtime             int
	VoteAverage         float64
	VoteCount           int
	Popularity          float64
	Adult               bool
	Status              string
	ProductionCompanies []string
	ProductionCountries []string
	SpokenLanguages     []string

	// Movie-only
	ReleaseDate         string
	Budget              int
	Revenue             int
	BelongsToCollection string
	IMDBID              string

	// TV-only
	Networks         []string
	OriginCountry    []string
	FirstAirDate     string
	LastAirDate      string
	Type             string
	InProduction     bool
	NumberOfSeasons  int
	NumberOfEpisodes int
	CreatedBy        []string
}

// FieldGroup classifies a field by where its data comes from.
type FieldGroup int

const (
	GroupA             FieldGroup = iota // request event
	GroupB                               // primary TMDB call
	GroupCKeywords                       // /movie|tv/{id}/keywords
	GroupCContentRating                  // /movie|tv/{id}/{release_dates|content_ratings}
)

// fieldRegistry maps every known field name to its FieldGroup.
var fieldRegistry = map[string]FieldGroup{
	// Group A — event payload
	"mediaType":        GroupA,
	"libraryId":        GroupA,
	"year":             GroupA,
	"decade":           GroupA,
	"requesterUserId":  GroupA,
	"requesterIsAdmin": GroupA,
	"title":            GroupA,
	"tmdbId":           GroupA,

	// Group B common — both movie and tv
	"original_language":    GroupB,
	"original_title":       GroupB,
	"genres":               GroupB,
	"runtime":              GroupB,
	"vote_average":         GroupB,
	"vote_count":           GroupB,
	"popularity":           GroupB,
	"adult":                GroupB,
	"status":               GroupB,
	"production_companies": GroupB,
	"production_countries": GroupB,
	"spoken_languages":     GroupB,

	// Group B movie-only
	"release_date":          GroupB,
	"budget":                GroupB,
	"revenue":               GroupB,
	"belongs_to_collection": GroupB,
	"imdb_id":               GroupB,

	// Group B tv-only
	"networks":          GroupB,
	"origin_country":    GroupB,
	"first_air_date":    GroupB,
	"last_air_date":     GroupB,
	"type":              GroupB,
	"in_production":     GroupB,
	"number_of_seasons": GroupB,
	"number_of_episodes": GroupB,
	"created_by":        GroupB,

	// Group C
	"keywords":       GroupCKeywords,
	"content_rating": GroupCContentRating,
}

// movieOnlyFields is the set of Group B fields only valid for MediaType=="movie".
var movieOnlyFields = map[string]struct{}{
	"release_date":          {},
	"budget":                {},
	"revenue":               {},
	"belongs_to_collection": {},
	"imdb_id":               {},
}

// tvOnlyFields is the set of Group B fields only valid for MediaType=="tv".
var tvOnlyFields = map[string]struct{}{
	"networks":           {},
	"origin_country":     {},
	"first_air_date":     {},
	"last_air_date":      {},
	"type":               {},
	"in_production":      {},
	"number_of_seasons":  {},
	"number_of_episodes": {},
	"created_by":         {},
}

// FieldGroupOf returns the group for a known field name.
// Returns (0, false) if the field is not registered.
func FieldGroupOf(name string) (FieldGroup, bool) {
	g, ok := fieldRegistry[name]
	return g, ok
}

// GetField returns (value, true) when the field is known AND its data group is
// loaded in ctx. Returns (nil, false) when the field is unknown, or when its
// data group hasn't been loaded. Kind-only fields (e.g., "networks" on a movie
// context) also return (nil, false).
func GetField(ctx Context, name string) (any, bool) {
	group, ok := fieldRegistry[name]
	if !ok {
		return nil, false
	}

	switch group {
	case GroupA:
		return getGroupAField(ctx, name)
	case GroupB:
		return getGroupBField(ctx, name)
	case GroupCKeywords:
		if ctx.Keywords == nil {
			return nil, false
		}
		return ctx.Keywords, true
	case GroupCContentRating:
		if ctx.ContentRating == "" {
			return nil, false
		}
		return ctx.ContentRating, true
	}
	return nil, false
}

func getGroupAField(ctx Context, name string) (any, bool) {
	switch name {
	case "mediaType":
		return ctx.Event.MediaType, true
	case "libraryId":
		return ctx.Event.LibraryID, true
	case "year":
		return ctx.Event.Year, true
	case "decade":
		y := ctx.Event.Year
		return y - (y % 10), true
	case "requesterUserId":
		return ctx.Event.RequesterUserID, true
	case "requesterIsAdmin":
		return ctx.Event.RequesterIsAdmin, true
	case "title":
		return ctx.Event.Title, true
	case "tmdbId":
		return ctx.Event.TMDBID, true
	}
	return nil, false
}

func getGroupBField(ctx Context, name string) (any, bool) {
	if ctx.Primary == nil {
		return nil, false
	}

	// Kind-only check: movie-only fields on a TV context
	if _, isMovieOnly := movieOnlyFields[name]; isMovieOnly {
		if ctx.Event.MediaType != "movie" {
			return nil, false
		}
	}

	// Kind-only check: tv-only fields on a movie context
	if _, isTVOnly := tvOnlyFields[name]; isTVOnly {
		if ctx.Event.MediaType != "tv" {
			return nil, false
		}
	}

	switch name {
	// Common
	case "original_language":
		return ctx.Primary.OriginalLanguage, true
	case "original_title":
		return ctx.Primary.OriginalTitle, true
	case "genres":
		return ctx.Primary.Genres, true
	case "runtime":
		return ctx.Primary.Runtime, true
	case "vote_average":
		return ctx.Primary.VoteAverage, true
	case "vote_count":
		return ctx.Primary.VoteCount, true
	case "popularity":
		return ctx.Primary.Popularity, true
	case "adult":
		return ctx.Primary.Adult, true
	case "status":
		return ctx.Primary.Status, true
	case "production_companies":
		return ctx.Primary.ProductionCompanies, true
	case "production_countries":
		return ctx.Primary.ProductionCountries, true
	case "spoken_languages":
		return ctx.Primary.SpokenLanguages, true
	// Movie-only
	case "release_date":
		return ctx.Primary.ReleaseDate, true
	case "budget":
		return ctx.Primary.Budget, true
	case "revenue":
		return ctx.Primary.Revenue, true
	case "belongs_to_collection":
		return ctx.Primary.BelongsToCollection, true
	case "imdb_id":
		return ctx.Primary.IMDBID, true
	// TV-only
	case "networks":
		return ctx.Primary.Networks, true
	case "origin_country":
		return ctx.Primary.OriginCountry, true
	case "first_air_date":
		return ctx.Primary.FirstAirDate, true
	case "last_air_date":
		return ctx.Primary.LastAirDate, true
	case "type":
		return ctx.Primary.Type, true
	case "in_production":
		return ctx.Primary.InProduction, true
	case "number_of_seasons":
		return ctx.Primary.NumberOfSeasons, true
	case "number_of_episodes":
		return ctx.Primary.NumberOfEpisodes, true
	case "created_by":
		return ctx.Primary.CreatedBy, true
	}
	return nil, false
}
