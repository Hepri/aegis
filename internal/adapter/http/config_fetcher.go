package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// HTTPConfigFetcher fetches config via long-poll from server
type HTTPConfigFetcher struct {
	baseURL        string
	clientID       string
	clientVersion  string
	client         *http.Client
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

// SetClientVersion sets the binary version reported on each long-poll.
func (f *HTTPConfigFetcher) SetClientVersion(v string) {
	f.clientVersion = v
}

// FetchConfig long-polls until config changes. If version is not empty, sends it so server
// can respond immediately when config version differs.
func (f *HTTPConfigFetcher) FetchConfig(ctx context.Context, version string) (*domain.ClientConfig, error) {
	q := url.Values{}
	q.Set("client_id", f.clientID)
	if version != "" {
		q.Set("version", version)
	}
	if f.clientVersion != "" {
		q.Set("client_version", f.clientVersion)
	}
	reqURL := fmt.Sprintf("%s/api/config?%s", f.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
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
