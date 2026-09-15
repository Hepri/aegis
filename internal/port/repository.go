package port

import (
	"context"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// BlockRequest is a block range [Start, Until]
type BlockRequest struct {
	ID     string    `json:"id"`
	UserID string    `json:"user_id,omitempty"` // empty = block all users
	Start  time.Time `json:"start"`
	Until  time.Time `json:"until"`
}

// TemporaryAccessRequest grants access to user from Start until Until
type TemporaryAccessRequest struct {
	ID     string    `json:"id"`
	UserID string    `json:"user_id"`
	Start  time.Time `json:"start"`
	Until  time.Time `json:"until"`
	Source string    `json:"source,omitempty"` // domain.TempAccessSourceEarn for wallet purchases
}

// ClientState holds persistent and ephemeral data for a client
type ClientState struct {
	ID                      string
	Name                    string
	Users                   []domain.User
	BlockRequests           []BlockRequest           // last 10, persisted
	TemporaryAccessRequests []TemporaryAccessRequest // last 10, persisted
	EarnTasks               []domain.EarnTask        // shared task list (may be empty)
	EarnSettings            domain.EarnSettings
	EarnLog                 []domain.EarnLogEntry // recent wallet/challenge ops
	LastSentIntervals       map[string][]domain.AllowedInterval
	LastSentVersion         string
	ComputedConfig          *domain.ClientConfig // precomputed intervals for today+tomorrow
}

// ConfigRepository persists and retrieves client configuration
type ConfigRepository interface {
	// GetClient returns client state by ID, nil if not found
	GetClient(ctx context.Context, clientID string) (*ClientState, error)

	// GetAllClients returns all clients
	GetAllClients(ctx context.Context) ([]*ClientState, error)

	// SaveClient persists client state
	SaveClient(ctx context.Context, client *ClientState) error

	// DeleteClient removes client
	DeleteClient(ctx context.Context, clientID string) error

	// AddUser adds user to client
	AddUser(ctx context.Context, clientID string, user domain.User) error

	// UpdateUserSchedule updates schedule for user
	UpdateUserSchedule(ctx context.Context, clientID, userID string, schedule domain.DaySchedule) error

	// DeleteUser removes user from client
	DeleteUser(ctx context.Context, clientID, userID string) error

	// GrantTemporaryAccess adds temporary access request, keeps last 10
	GrantTemporaryAccess(ctx context.Context, clientID, userID string, until time.Time) error

	// BlockClient adds block request, keeps last 10 (userID empty = block all)
	BlockClient(ctx context.Context, clientID, userID string, start, until time.Time) error

	// DeleteBlockRequest removes block by ID
	DeleteBlockRequest(ctx context.Context, clientID, requestID string) error

	// DeleteTemporaryAccessRequest removes temp access by ID
	DeleteTemporaryAccessRequest(ctx context.Context, clientID, requestID string) error

	// UpdateEarnSettings updates earn settings for a client
	UpdateEarnSettings(ctx context.Context, clientID string, settings domain.EarnSettings) error

	// UpsertEarnTask creates or updates a custom (client-owned) earn task.
	UpsertEarnTask(ctx context.Context, clientID string, task domain.EarnTask) (domain.EarnTask, error)

	// DeleteEarnTask removes a custom earn task. Builtin tasks cannot be deleted.
	DeleteEarnTask(ctx context.Context, clientID, taskID string) error

	// IssueEarnChallenge returns next public task (bank or generated math).
	IssueEarnChallenge(ctx context.Context, clientID, userID string) (*domain.EarnPublicTask, int, error) // task, lockRemainingSec, err

	// AnswerEarnTask checks answer, credits wallet or applies wrong-answer lock.
	AnswerEarnTask(ctx context.Context, clientID, userID, taskID, answer string) (domain.EarnAnswerResult, error)

	// SkipEarnChallenge replaces the current question and applies the wrong-answer lock timer (no streak penalty).
	SkipEarnChallenge(ctx context.Context, clientID, userID, taskID string) (domain.EarnAnswerResult, error)

	// RedeemEarnMinutes spends wallet minutes and grants temporary access from now.
	RedeemEarnMinutes(ctx context.Context, clientID, userID string, minutes int) (domain.EarnRedeemResult, error)

	// RefundEarnSession ends active earn-bought temp access and credits unused minutes back.
	RefundEarnSession(ctx context.Context, clientID, userID string) (domain.EarnRefundResult, error)

	// ClearEarnBalances zeroes earn wallet balances for all users on a client.
	ClearEarnBalances(ctx context.Context, clientID string) error

	// AdjustEarnBalance adds (positive) or removes (negative) minutes from one user's wallet.
	AdjustEarnBalance(ctx context.Context, clientID, userID string, delta int) (balance int, err error)

	// ListEarnLog returns recent earn wallet/challenge operations (newest first).
	ListEarnLog(ctx context.Context, clientID string, limit int) ([]domain.EarnLogEntry, error)

	// UpdateLastSent updates last sent intervals for change detection
	UpdateLastSent(ctx context.Context, clientID string, intervals map[string][]domain.AllowedInterval) error

	// IncrementConfigVersion increments config version when admin makes changes
	IncrementConfigVersion(ctx context.Context, clientID string) error

	// Subscribe returns a channel that receives when config may have changed for client
	Subscribe(ctx context.Context, clientID string) <-chan struct{}
}
