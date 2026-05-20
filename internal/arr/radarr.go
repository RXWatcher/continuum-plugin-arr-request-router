package arr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type Radarr struct {
	httpClient
}

func NewRadarr(baseURL, apiKey string) *Radarr {
	return &Radarr{httpClient: newHTTPClient(baseURL, apiKey)}
}

type Movie struct {
	ID                  int    `json:"id"`
	Title               string `json:"title"`
	Year                int    `json:"year"`
	TMDBID              int    `json:"tmdbId"`
	HasFile             bool   `json:"hasFile"`
	Monitored           bool   `json:"monitored"`
	QualityProfileID    int    `json:"qualityProfileId"`
	RootFolderPath      string `json:"rootFolderPath"`
	MinimumAvailability string `json:"minimumAvailability,omitempty"`
}

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

var ErrNotFound = errors.New("arr: not found")
