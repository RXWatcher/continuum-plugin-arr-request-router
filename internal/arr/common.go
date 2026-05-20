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

const defaultTimeout = 10 * time.Second
const maxResponseBytes = 10 << 20 // 10 MiB

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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
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

type QueueItem struct {
	ID                    int    `json:"id"`
	MovieID               int    `json:"movieId,omitempty"`
	SeriesID              int    `json:"seriesId,omitempty"`
	Status                string `json:"status"`
	TrackedDownloadStatus string `json:"trackedDownloadStatus,omitempty"`
}

type queueEnvelope struct {
	Records []QueueItem `json:"records"`
}

type RootFolder struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

type QualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type LanguageProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type SystemStatusResponse struct {
	Version      string `json:"version"`
	InstanceName string `json:"instanceName"`
	AppName      string `json:"appName"`
	Branch       string `json:"branch"`
}

var defaultProbeClient = &http.Client{Timeout: defaultTimeout}

func SystemStatus(ctx context.Context, baseURL, apiKey string) (SystemStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimSlash(baseURL)+"/api/v3/system/status", nil)
	if err != nil {
		return SystemStatusResponse{}, err
	}
	req.Header.Set("X-Api-Key", apiKey)
	resp, err := defaultProbeClient.Do(req)
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
