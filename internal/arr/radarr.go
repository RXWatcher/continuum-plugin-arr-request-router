package arr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Radarr is the v3 Radarr client. Construct with NewRadarr.
type Radarr struct {
	httpClient
}

func NewRadarr(baseURL, apiKey string) *Radarr {
	return &Radarr{httpClient: newHTTPClient(baseURL, apiKey)}
}

// Movie is the subset of /api/v3/movie we read from Radarr.
type Movie struct {
	ID                int    `json:"id"`
	Title             string `json:"title"`
	Year              int    `json:"year"`
	TMDBID            int    `json:"tmdbId"`
	HasFile           bool   `json:"hasFile"`
	Monitored         bool   `json:"monitored"`
	QualityProfileID  int    `json:"qualityProfileId"`
	RootFolderPath    string `json:"rootFolderPath"`
	MinimumAvailability string `json:"minimumAvailability,omitempty"`
}

// AddMovieRequest is the request body for POST /api/v3/movie.
type AddMovieRequest struct {
	Title               string         `json:"title"`
	Year                int            `json:"year"`
	TMDBID              int            `json:"tmdbId"`
	QualityProfileID    int            `json:"qualityProfileId"`
	RootFolderPath      string         `json:"rootFolderPath"`
	Monitored           bool           `json:"monitored"`
	MinimumAvailability string         `json:"minimumAvailability"`
	AddOptions          map[string]any `json:"addOptions"`
}

// LookupMovieByTMDB queries the lookup endpoint by TMDB id and returns the
// canonical title/year. Returns ErrNotFound if Radarr can't find it.
func (r *Radarr) LookupMovieByTMDB(ctx context.Context, tmdbID int) (Movie, error) {
	q := url.Values{}
	q.Set("term", fmt.Sprintf("tmdb:%d", tmdbID))
	body, err := r.do(ctx, http.MethodGet, "/api/v3/movie/lookup", q, nil)
	if err != nil {
		return Movie{}, err
	}
	var hits []Movie
	if err := json.Unmarshal(body, &hits); err != nil {
		return Movie{}, fmt.Errorf("decode lookup: %w", err)
	}
	for _, m := range hits {
		if m.TMDBID == tmdbID {
			return m, nil
		}
	}
	if len(hits) == 0 {
		return Movie{}, ErrNotFound
	}
	return hits[0], nil
}

// AddMovie creates a movie in Radarr. Returns the created movie (with id).
// 409 conflict means the movie already exists; callers should treat that as
// "already submitted".
func (r *Radarr) AddMovie(ctx context.Context, req AddMovieRequest) (Movie, error) {
	body, err := r.do(ctx, http.MethodPost, "/api/v3/movie", nil, req)
	if err != nil {
		return Movie{}, err
	}
	var m Movie
	if err := json.Unmarshal(body, &m); err != nil {
		return Movie{}, fmt.Errorf("decode add: %w", err)
	}
	return m, nil
}

// GetMovie fetches a movie by Radarr's internal id.
func (r *Radarr) GetMovie(ctx context.Context, id int) (Movie, error) {
	body, err := r.do(ctx, http.MethodGet, fmt.Sprintf("/api/v3/movie/%d", id), nil, nil)
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
			return Movie{}, ErrNotFound
		}
		return Movie{}, err
	}
	var m Movie
	if err := json.Unmarshal(body, &m); err != nil {
		return Movie{}, fmt.Errorf("decode get: %w", err)
	}
	return m, nil
}

// DeleteMovie removes a movie. Files and exclusion list are preserved.
func (r *Radarr) DeleteMovie(ctx context.Context, id int) error {
	q := url.Values{}
	q.Set("deleteFiles", "false")
	q.Set("addImportListExclusion", "false")
	_, err := r.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v3/movie/%d", id), q, nil)
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

// QueueByMovie returns queue items for the given movie id.
func (r *Radarr) QueueByMovie(ctx context.Context, movieID int) ([]QueueItem, error) {
	q := url.Values{}
	q.Set("movieId", intParam(movieID))
	body, err := r.do(ctx, http.MethodGet, "/api/v3/queue", q, nil)
	if err != nil {
		return nil, err
	}
	var env queueEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode queue: %w", err)
	}
	return env.Records, nil
}

// RootFolders lists configured root folders. Used to default the root path.
func (r *Radarr) RootFolders(ctx context.Context) ([]RootFolder, error) {
	body, err := r.do(ctx, http.MethodGet, "/api/v3/rootfolder", nil, nil)
	if err != nil {
		return nil, err
	}
	var folders []RootFolder
	if err := json.Unmarshal(body, &folders); err != nil {
		return nil, fmt.Errorf("decode rootfolder: %w", err)
	}
	return folders, nil
}

// QualityProfiles lists configured quality profiles.
func (r *Radarr) QualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	body, err := r.do(ctx, http.MethodGet, "/api/v3/qualityprofile", nil, nil)
	if err != nil {
		return nil, err
	}
	var profiles []QualityProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("decode qualityprofile: %w", err)
	}
	return profiles, nil
}

// ErrNotFound is returned when a lookup or get cannot find the resource.
var ErrNotFound = errors.New("arr: not found")
