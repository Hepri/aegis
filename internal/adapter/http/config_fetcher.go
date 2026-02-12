package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// HTTPConfigFetcher fetches config via long-poll from server
type HTTPConfigFetcher struct {
	baseURL  string
	clientID string
	client   *http.Client
}

func NewHTTPConfigFetcher(baseURL, clientID string) *HTTPConfigFetcher {
	return &HTTPConfigFetcher{
		baseURL:  baseURL,
		clientID: clientID,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// FetchConfig long-polls until config changes. If version is not empty, sends it so server
// can respond immediately when config version differs.
func (f *HTTPConfigFetcher) FetchConfig(ctx context.Context, version string) (*domain.ClientConfig, error) {
	url := fmt.Sprintf("%s/api/config?client_id=%s", f.baseURL, f.clientID)
	if version != "" {
		url = fmt.Sprintf("%s&version=%s", url, version)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	var cfg domain.ClientConfig
	return &cfg, json.NewDecoder(resp.Body).Decode(&cfg)
}
