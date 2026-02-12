package port

import (
	"context"

	"github.com/aegis/parental-control/internal/domain"
)

// ConfigFetcher fetches client config from server (long-poll)
type ConfigFetcher interface {
	// FetchConfig long-polls until config changes, returns new config
	FetchConfig(ctx context.Context, clientID string) (*domain.ClientConfig, error)
}
