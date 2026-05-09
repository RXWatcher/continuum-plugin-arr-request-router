// Package arr provides minimal Radarr (movies) and Sonarr (TV) HTTP clients.
// Both *arr products share the v3 API conventions: X-Api-Key header,
// JSON request/response, and a small set of endpoints we care about
// (lookup, add, delete, get, queue, root-folder, quality-profile).
package arr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// defaultTimeout caps every *arr HTTP call. *arr instances are expected to be
// on the same LAN, so 10s is generous.
const defaultTimeout = 10 * time.Second

// httpClient is the common HTTP plumbing shared by the Radarr and Sonarr
// clients. It does not embed a base URL or api key — the embedding client
// supplies those per call. Construction is the same for both, so we share
// the constructor too.
type httpClient struct {
	baseURL string
	apiKey  string
	hc      *http.Client
}

func newHTTPClient(baseURL, apiKey string) httpClient {
	return httpClient{
		baseURL: trimSlash(baseURL),
		apiKey:  apiKey,
		hc:      &http.Client{Timeout: defaultTimeout},
	}
}

// HTTPError carries the status code and body of a non-2xx *arr response so
// callers can inspect 409 (already exists) etc.
type HTTPError struct {
	StatusCode int
	Body       string
	URL        string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("arr: %s returned %d", e.URL, e.StatusCode)
	}
	return fmt.Sprintf("arr: %s returned %d: %s", e.URL, e.StatusCode, e.Body)
}

// IsConflict reports whether err is an *arr 409 (already exists). Callers map
// this to "treat as already submitted".
func IsConflict(err error) bool {
	he, ok := err.(*HTTPError)
	return ok && he.StatusCode == http.StatusConflict
}

func (c httpClient) do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			URL:        u,
		}
	}
	return respBody, nil
}

// QueueItem is the subset of /queue we care about. *arr returns a paginated
// envelope; the real queue items live under "records".
type QueueItem struct {
	ID                int    `json:"id"`
	MovieID           int    `json:"movieId,omitempty"`
	SeriesID          int    `json:"seriesId,omitempty"`
	Status            string `json:"status"`
	TrackedDownloadStatus string `json:"trackedDownloadStatus,omitempty"`
}

type queueEnvelope struct {
	Records []QueueItem `json:"records"`
}

// RootFolder is the subset of /rootfolder we use.
type RootFolder struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

// QualityProfile is the subset of /qualityprofile we use.
type QualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// LanguageProfile is Sonarr-only.
type LanguageProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// defaultHTTPClient is a package-level client for one-shot probes (e.g.
// SystemStatus) that don't belong to a specific Radarr/Sonarr instance.
var defaultHTTPClient = &http.Client{Timeout: defaultTimeout}

// SystemStatusResponse mirrors the GET /api/v3/system/status payload.
// Both Radarr and Sonarr v3 expose this with the same shape.
type SystemStatusResponse struct {
	Version      string `json:"version"`
	InstanceName string `json:"instanceName"`
	AppName      string `json:"appName"`  // Sonarr v4 returns this too; harmless on Radarr
	Branch       string `json:"branch"`
}

// SystemStatus probes a Radarr or Sonarr instance with the given URL + API
// key. Returns the parsed response on success, or a descriptive error on
// any non-2xx response or transport failure. Used by the admin
// "Test connection" UI to verify a key before persisting.
func SystemStatus(ctx context.Context, baseURL, apiKey string) (SystemStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", trimSlash(baseURL)+"/api/v3/system/status", nil)
	if err != nil {
		return SystemStatusResponse{}, err
	}
	req.Header.Set("X-Api-Key", apiKey)
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return SystemStatusResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return SystemStatusResponse{}, fmt.Errorf("system/status: %d", resp.StatusCode)
	}
	var out SystemStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SystemStatusResponse{}, err
	}
	return out, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func intParam(v int) string { return strconv.Itoa(v) }
