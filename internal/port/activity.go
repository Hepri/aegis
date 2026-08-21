package port

import (
	"context"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// ActivityStore persists and reads client activity events.
type ActivityStore interface {
	AppendEvents(ctx context.Context, clientID string, events []domain.ActivityEvent) error
	ReadDayEvents(ctx context.Context, clientID string, day time.Time) ([]domain.ActivityEvent, error)
	CleanupOlderThan(ctx context.Context, retentionDays int) error
}

// ClientPresence is last contact info from a client.
type ClientPresence struct {
	LastSeen       time.Time `json:"last_seen"`
	ClientVersion  string    `json:"client_version,omitempty"`
}

// PresenceStore tracks when clients last contacted the server.
type PresenceStore interface {
	TouchPresence(ctx context.Context, clientID, clientVersion string) error
	GetPresence(ctx context.Context, clientID string) (ClientPresence, bool)
	GetAllPresence(ctx context.Context) map[string]ClientPresence
}
