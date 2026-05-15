package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
)

// Client is an HTTP client for the TMDB v3 API.
// It handles Primary lookup, Keywords, and ContentRating for both movie and tv.
type Client struct {
	baseURL  string
	apiKey   string
	language string
	http     *http.Client
}

// New returns a new Client. If language is empty it defaults to "en-US".
func New(baseURL, apiKey, language string) *Client {
	if language == "" {
		language = "en-US"
	}
	return &Client{
		baseURL:  baseURL,
		apiKey:   apiKey,
		language: language,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// ---------------------------------------------------------------------------
// Public methods
// ---------------------------------------------------------------------------

// Primary fetches the top-level metadata for a movie or TV show and maps it
// to a routing.TMDBPrimary. mediaType must be "movie" or "tv".
func (c *Client) Primary(ctx context.Context, mediaType string, tmdbID int) (*routing.TMDBPrimary, error) {
	switch mediaType {
	case "movie":
		var m primaryMovie
		if err := c.getJSON(ctx, fmt.Sprintf("/movie/%d", tmdbID), &m); err != nil {
			return nil, err
		}
		return movieToPrimary(m), nil
	case "tv":
		var t primaryTV
		if err := c.getJSON(ctx, fmt.Sprintf("/tv/%d", tmdbID), &t); err != nil {
			return nil, err
		}
		return tvToPrimary(t), nil
	default:
		return nil, fmt.Errorf("tmdb: unknown mediaType %q", mediaType)
	}
}

// Keywords fetches keyword names for a movie or TV show.
// TMDB returns different JSON shapes for each type:
//
//	movie  → {"keywords":[{"name":"…"},…]}
//	tv     → {"results":[{"name":"…"},…]}
func (c *Client) Keywords(ctx context.Context, mediaType string, tmdbID int) ([]string, error) {
	switch mediaType {
	case "movie":
		var resp struct {
			Keywords []nameObject `json:"keywords"`
		}
		if err := c.getJSON(ctx, fmt.Sprintf("/movie/%d/keywords", tmdbID), &resp); err != nil {
			return nil, err
		}
		return extractNames(resp.Keywords), nil
	case "tv":
		var resp struct {
			Results []nameObject `json:"results"`
		}
		if err := c.getJSON(ctx, fmt.Sprintf("/tv/%d/keywords", tmdbID), &resp); err != nil {
			return nil, err
		}
		return extractNames(resp.Results), nil
	default:
		return nil, fmt.Errorf("tmdb: unknown mediaType %q", mediaType)
	}
}

// ContentRating fetches the US content rating for a movie or TV show.
// Returns ("", nil) when no US entry is found or no non-empty certification
// exists for that entry.
func (c *Client) ContentRating(ctx context.Context, mediaType string, tmdbID int) (string, error) {
	switch mediaType {
	case "movie":
		var resp releaseDatesResponse
		if err := c.getJSON(ctx, fmt.Sprintf("/movie/%d/release_dates", tmdbID), &resp); err != nil {
			return "", err
		}
		for _, entry := range resp.Results {
			if entry.ISO31661 != "US" {
				continue
			}
			for _, rd := range entry.ReleaseDates {
				if rd.Certification != "" {
					return rd.Certification, nil
				}
			}
		}
		return "", nil

	case "tv":
		var resp contentRatingsResponse
		if err := c.getJSON(ctx, fmt.Sprintf("/tv/%d/content_ratings", tmdbID), &resp); err != nil {
			return "", err
		}
		for _, entry := range resp.Results {
			if entry.ISO31661 == "US" {
				return entry.Rating, nil
			}
		}
		return "", nil

	default:
		return "", fmt.Errorf("tmdb: unknown mediaType %q", mediaType)
	}
}

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("tmdb: parse URL: %w", err)
	}
	q := u.Query()
	q.Set("api_key", c.apiKey)
	q.Set("language", c.language)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("tmdb %s: %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("tmdb: decode response: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Private response shapes
// ---------------------------------------------------------------------------

type nameObject struct {
	Name string `json:"name"`
}

// primaryMovie is the raw JSON shape from GET /movie/{id}.
type primaryMovie struct {
	OriginalLanguage    string       `json:"original_language"`
	OriginalTitle       string       `json:"original_title"`
	Genres              []nameObject `json:"genres"`
	Runtime             int          `json:"runtime"`
	VoteAverage         float64      `json:"vote_average"`
	VoteCount           int          `json:"vote_count"`
	Popularity          float64      `json:"popularity"`
	Adult               bool         `json:"adult"`
	Status              string       `json:"status"`
	ProductionCompanies []nameObject `json:"production_companies"`
	ProductionCountries []struct {
		ISO31661 string `json:"iso_3166_1"`
	} `json:"production_countries"`
	SpokenLanguages []struct {
		ISO6391 string `json:"iso_639_1"`
	} `json:"spoken_languages"`
	ReleaseDate         string `json:"release_date"`
	Budget              int    `json:"budget"`
	Revenue             int    `json:"revenue"`
	BelongsToCollection *struct {
		Name string `json:"name"`
	} `json:"belongs_to_collection"`
	IMDBID string `json:"imdb_id"`
}

// primaryTV is the raw JSON shape from GET /tv/{id}.
type primaryTV struct {
	OriginalLanguage    string       `json:"original_language"`
	OriginalName        string       `json:"original_name"`
	Genres              []nameObject `json:"genres"`
	EpisodeRunTime      []int        `json:"episode_run_time"`
	VoteAverage         float64      `json:"vote_average"`
	VoteCount           int          `json:"vote_count"`
	Popularity          float64      `json:"popularity"`
	Adult               bool         `json:"adult"`
	Status              string       `json:"status"`
	ProductionCompanies []nameObject `json:"production_companies"`
	ProductionCountries []struct {
		ISO31661 string `json:"iso_3166_1"`
	} `json:"production_countries"`
	SpokenLanguages []struct {
		ISO6391 string `json:"iso_639_1"`
	} `json:"spoken_languages"`
	Networks      []nameObject `json:"networks"`
	OriginCountry []string     `json:"origin_country"`
	FirstAirDate  string       `json:"first_air_date"`
	LastAirDate   string       `json:"last_air_date"`
	Type          string       `json:"type"`
	InProduction  bool         `json:"in_production"`
	NumberOfSeasons  int          `json:"number_of_seasons"`
	NumberOfEpisodes int          `json:"number_of_episodes"`
	CreatedBy        []nameObject `json:"created_by"`
}

// releaseDatesResponse is the shape of GET /movie/{id}/release_dates.
type releaseDatesResponse struct {
	Results []struct {
		ISO31661 string `json:"iso_3166_1"`
		ReleaseDates []struct {
			Certification string `json:"certification"`
		} `json:"release_dates"`
	} `json:"results"`
}

// contentRatingsResponse is the shape of GET /tv/{id}/content_ratings.
type contentRatingsResponse struct {
	Results []struct {
		ISO31661 string `json:"iso_3166_1"`
		Rating   string `json:"rating"`
	} `json:"results"`
}

// ---------------------------------------------------------------------------
// Mapping helpers
// ---------------------------------------------------------------------------

func movieToPrimary(m primaryMovie) *routing.TMDBPrimary {
	p := &routing.TMDBPrimary{
		MediaType:           "movie",
		OriginalLanguage:    m.OriginalLanguage,
		OriginalTitle:       m.OriginalTitle,
		Genres:              extractNames(m.Genres),
		Runtime:             m.Runtime,
		VoteAverage:         m.VoteAverage,
		VoteCount:           m.VoteCount,
		Popularity:          m.Popularity,
		Adult:               m.Adult,
		Status:              m.Status,
		ProductionCompanies: extractNames(m.ProductionCompanies),
		ProductionCountries: extractISO31661(m.ProductionCountries),
		SpokenLanguages:     extractISO6391(m.SpokenLanguages),
		ReleaseDate:         m.ReleaseDate,
		Budget:              m.Budget,
		Revenue:             m.Revenue,
		IMDBID:              m.IMDBID,
	}
	if m.BelongsToCollection != nil {
		p.BelongsToCollection = m.BelongsToCollection.Name
	}
	return p
}

func tvToPrimary(t primaryTV) *routing.TMDBPrimary {
	runtime := 0
	if len(t.EpisodeRunTime) > 0 {
		runtime = t.EpisodeRunTime[0]
	}
	return &routing.TMDBPrimary{
		MediaType:           "tv",
		OriginalLanguage:    t.OriginalLanguage,
		OriginalTitle:       t.OriginalName, // TV uses original_name
		Genres:              extractNames(t.Genres),
		Runtime:             runtime,
		VoteAverage:         t.VoteAverage,
		VoteCount:           t.VoteCount,
		Popularity:          t.Popularity,
		Adult:               t.Adult,
		Status:              t.Status,
		ProductionCompanies: extractNames(t.ProductionCompanies),
		ProductionCountries: extractISO31661(t.ProductionCountries),
		SpokenLanguages:     extractISO6391(t.SpokenLanguages),
		Networks:            extractNames(t.Networks),
		OriginCountry:       t.OriginCountry,
		FirstAirDate:        t.FirstAirDate,
		LastAirDate:         t.LastAirDate,
		Type:                t.Type,
		InProduction:        t.InProduction,
		NumberOfSeasons:     t.NumberOfSeasons,
		NumberOfEpisodes:    t.NumberOfEpisodes,
		CreatedBy:           extractNames(t.CreatedBy),
	}
}

func extractNames(items []nameObject) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Name
	}
	return out
}

func extractISO31661(items []struct {
	ISO31661 string `json:"iso_3166_1"`
}) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ISO31661
	}
	return out
}

func extractISO6391(items []struct {
	ISO6391 string `json:"iso_639_1"`
}) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ISO6391
	}
	return out
}
