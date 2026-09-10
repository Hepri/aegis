package domain

import "errors"

var (
	ErrClientNotFound      = errors.New("client not found")
	ErrUserNotFound        = errors.New("user not found")
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskAlreadySolved   = errors.New("task already solved")
	ErrDailyLimit          = errors.New("daily earn limit reached")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidMinutes      = errors.New("minutes must be positive")
)

// EarnTask is a server-side problem. Answer is never sent to the child UI.
type EarnTask struct {
	ID            string `json:"id"`
	Prompt        string `json:"prompt"`
	Answer        string `json:"answer"`
	RewardMinutes int    `json:"reward_minutes"`
	Enabled       bool   `json:"enabled"`
}

// EarnSettings are per-client defaults/limits for the earn wallet.
type EarnSettings struct {
	DefaultRewardMinutes int `json:"default_reward_minutes"`
	MaxEarnPerDay        int `json:"max_earn_per_day"`
}

// EarnDayStats tracks daily earn caps for one user.
type EarnDayStats struct {
	Date          string `json:"date"` // YYYY-MM-DD in server timezone
	EarnedMinutes int    `json:"earned_minutes"`
	SolvedCount   int    `json:"solved_count"`
}

// DefaultEarnSettings returns sensible defaults when unset.
func DefaultEarnSettings() EarnSettings {
	return EarnSettings{
		DefaultRewardMinutes: 5,
		MaxEarnPerDay:        120,
	}
}

// EffectiveReward returns task reward or settings default.
func EffectiveReward(task EarnTask, settings EarnSettings) int {
	if task.RewardMinutes > 0 {
		return task.RewardMinutes
	}
	if settings.DefaultRewardMinutes > 0 {
		return settings.DefaultRewardMinutes
	}
	return DefaultEarnSettings().DefaultRewardMinutes
}
