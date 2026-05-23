package arr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type Sonarr struct {
	httpClient
}

func NewSonarr(baseURL, apiKey string) *Sonarr {
	return &Sonarr{httpClient: newHTTPClient(baseURL, apiKey)}
}

type Series struct {
	ID                int    `json:"id"`
	Title             string `json:"title"`
	Year              int    `json:"year"`
	TMDBID            int    `json:"tmdbId"`
	TVDBID            int    `json:"tvdbId"`
	Monitored         bool   `json:"monitored"`
	QualityProfileID  int    `json:"qualityProfileId"`
	LanguageProfileID int    `json:"languageProfileId"`
	RootFolderPath    string `json:"rootFolderPath"`
	Statistics        struct {
		PercentOfEpisodes float64 `json:"percentOfEpisodes"`
		EpisodeFileCount  int     `json:"episodeFileCount"`
		EpisodeCount      int     `json:"episodeCount"`
	} `json:"statistics"`
}

type AddSeriesRequest struct {
	Title             string         `json:"title"`
	Year              int            `json:"year"`
	TMDBID            int            `json:"tmdbId"`
	TVDBID            int            `json:"tvdbId,omitempty"`
	QualityProfileID  int            `json:"qualityProfileId"`
	LanguageProfileID int            `json:"languageProfileId,omitempty"`
	RootFolderPath    string         `json:"rootFolderPath"`
	Monitored         bool           `json:"monitored"`
	SeasonFolder      bool           `json:"seasonFolder"`
	AddOptions        map[string]any `json:"addOptions"`
}

func (s *Sonarr) LookupSeriesByTMDB(ctx context.Context, tmdbID int, fallbackTerm string) (Series, error) {
	if hit, err := s.lookupSeries(ctx, fmt.Sprintf("tmdb:%d", tmdbID), tmdbID); err == nil {
		return hit, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Series{}, err
	}
	if fallbackTerm == "" {
		return Series{}, ErrNotFound
	}
	return s.lookupSeries(ctx, fallbackTerm, tmdbID)
}

func (s *Sonarr) lookupSeries(ctx context.Context, term string, tmdbID int) (Series, error) {
	q := url.Values{}
	q.Set("term", term)
	body, err := s.do(ctx, http.MethodGet, "/api/v3/series/lookup", q, nil)
	if err != nil {
		return Series{}, err
	}
	var hits []Series
	if err := json.Unmarshal(body, &hits); err != nil {
		return Series{}, fmt.Errorf("decode lookup: %w", err)
	}
	if tmdbID > 0 {
		for _, h := range hits {
			if h.TMDBID == tmdbID {
				return h, nil
			}
		}
	}
	if len(hits) == 0 {
		return Series{}, ErrNotFound
	}
	return hits[0], nil
}

func (s *Sonarr) AddSeries(ctx context.Context, req AddSeriesRequest) (Series, error) {
	body, err := s.do(ctx, http.MethodPost, "/api/v3/series", nil, req)
	if err != nil {
		return Series{}, err
	}
	var out Series
	if err := json.Unmarshal(body, &out); err != nil {
		return Series{}, fmt.Errorf("decode add: %w", err)
	}
	return out, nil
}

func (s *Sonarr) GetSeries(ctx context.Context, id int) (Series, error) {
	body, err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v3/series/%d", id), nil, nil)
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
			return Series{}, ErrNotFound
		}
		return Series{}, err
	}
	var out Series
	if err := json.Unmarshal(body, &out); err != nil {
		return Series{}, fmt.Errorf("decode get: %w", err)
	}
	return out, nil
}

func (s *Sonarr) DeleteSeries(ctx context.Context, id int) error {
	q := url.Values{}
	q.Set("deleteFiles", "false")
	q.Set("addImportListExclusion", "false")
	_, err := s.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v3/series/%d", id), q, nil)
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

// QueueBySeries returns only the queue records for the given Sonarr series
// id. The aggregated /queue endpoint can't be filtered server-side, so we
// pull the whole queue and match on seriesId here.
func (s *Sonarr) QueueBySeries(ctx context.Context, seriesID int) ([]QueueItem, error) {
	all, err := s.fetchAllQueue(ctx)
	if err != nil {
		return nil, err
	}
	var out []QueueItem
	for _, item := range all {
		if item.SeriesID == seriesID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Sonarr) RootFolders(ctx context.Context) ([]RootFolder, error) {
	body, err := s.do(ctx, http.MethodGet, "/api/v3/rootfolder", nil, nil)
	if err != nil {
		return nil, err
	}
	var folders []RootFolder
	if err := json.Unmarshal(body, &folders); err != nil {
		return nil, fmt.Errorf("decode rootfolder: %w", err)
	}
	return folders, nil
}

func (s *Sonarr) QualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	body, err := s.do(ctx, http.MethodGet, "/api/v3/qualityprofile", nil, nil)
	if err != nil {
		return nil, err
	}
	var profiles []QualityProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("decode qualityprofile: %w", err)
	}
	return profiles, nil
}

func (s *Sonarr) LanguageProfiles(ctx context.Context) ([]LanguageProfile, error) {
	body, err := s.do(ctx, http.MethodGet, "/api/v3/languageprofile", nil, nil)
	if err != nil {
		return nil, err
	}
	var profiles []LanguageProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("decode languageprofile: %w", err)
	}
	return profiles, nil
}
