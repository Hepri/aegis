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

// PresenceStore tracks when clients last contacted the server.
type PresenceStore interface {
	TouchLastSeen(ctx context.Context, clientID string) error
	GetLastSeen(ctx context.Context, clientID string) (time.Time, bool)
	GetAllLastSeen(ctx context.Context) map[string]time.Time
}
