package domain

import (
	"errors"
	"time"
)

var (
	ErrClientNotFound      = errors.New("client not found")
	ErrUserNotFound        = errors.New("user not found")
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskAlreadySolved   = errors.New("task already solved")
	ErrDailyLimit          = errors.New("daily earn limit reached")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidMinutes      = errors.New("minutes must be positive")
	ErrEarnLocked          = errors.New("earn locked after wrong answer")
)

const (
	EarnKindText   = "text"
	EarnKindChoice = "choice"
	EarnSourceBank = "bank"
	EarnSourceGen  = "generated"
)

// EarnTask is a persisted bank problem. Answer is never sent to the child UI.
type EarnTask struct {
	ID            string   `json:"id"`
	Prompt        string   `json:"prompt"`
	Answer        string   `json:"answer"`
	Choices       []string `json:"choices,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	RewardMinutes int      `json:"reward_minutes"`
	Enabled       bool     `json:"enabled"`
}

// EarnChallenge is the child's current unfinished question (persisted on the user).
// Reloading the page must return the same challenge until it is solved or replaced after a wrong streak.
type EarnChallenge struct {
	ID            string    `json:"id"`
	Prompt        string    `json:"prompt"`
	Answer        string    `json:"answer"`
	Choices       []string  `json:"choices,omitempty"`
	Kind          string    `json:"kind,omitempty"`
	RewardMinutes int       `json:"reward_minutes"`
	Source        string    `json:"source"`
	BankTaskID    string    `json:"bank_task_id,omitempty"`
	WrongCount    int       `json:"wrong_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// EarnPublicTask is what the child UI receives (no answer).
type EarnPublicTask struct {
	ID            string   `json:"id"`
	Prompt        string   `json:"prompt"`
	Kind          string   `json:"kind"`
	Choices       []string `json:"choices,omitempty"`
	RewardMinutes int      `json:"reward_minutes"`
	Source        string   `json:"source"`
	WrongCount    int      `json:"wrong_count,omitempty"`
}

// EarnAnswerResult is returned after checking an answer.
type EarnAnswerResult struct {
	Correct         bool `json:"correct"`
	Balance         int  `json:"balance_minutes"`
	LockSeconds     int  `json:"lock_seconds,omitempty"`
	RewardMinutes   int  `json:"reward_minutes,omitempty"`
	WrongCount      int  `json:"wrong_count,omitempty"`
	WrongStreakMax  int  `json:"wrong_streak_max,omitempty"`
	PenaltyMinutes  int  `json:"penalty_minutes,omitempty"`
	ReplaceQuestion bool `json:"replace_question,omitempty"` // true → client must fetch next
}

// EarnSettings are per-client defaults/limits for the earn wallet.
type EarnSettings struct {
	DefaultRewardMinutes     int  `json:"default_reward_minutes"`
	MaxEarnPerDay            int  `json:"max_earn_per_day"`
	WrongLockSeconds         int  `json:"wrong_lock_seconds"`
	WrongStreakLimit         int  `json:"wrong_streak_limit"`          // N wrong answers → penalty + new question
	WrongStreakPenaltyMinutes int `json:"wrong_streak_penalty_minutes"` // minutes removed after N wrongs
	MathGeneratorEnabled     bool `json:"math_generator_enabled"`
}

// EarnDayStats tracks daily earn caps for one user.
type EarnDayStats struct {
	Date          string `json:"date"`
	EarnedMinutes int    `json:"earned_minutes"`
	SolvedCount   int    `json:"solved_count"`
}

func DefaultEarnSettings() EarnSettings {
	return EarnSettings{
		DefaultRewardMinutes:      1,
		MaxEarnPerDay:             120,
		WrongLockSeconds:          15,
		WrongStreakLimit:          3,
		WrongStreakPenaltyMinutes: 1,
		MathGeneratorEnabled:      true,
	}
}

func NormalizeEarnSettings(s EarnSettings) EarnSettings {
	d := DefaultEarnSettings()
	if s.DefaultRewardMinutes <= 0 {
		s.DefaultRewardMinutes = d.DefaultRewardMinutes
	}
	if s.MaxEarnPerDay <= 0 {
		s.MaxEarnPerDay = d.MaxEarnPerDay
	}
	if s.WrongLockSeconds <= 0 {
		s.WrongLockSeconds = d.WrongLockSeconds
	}
	if s.WrongStreakLimit <= 0 {
		s.WrongStreakLimit = d.WrongStreakLimit
	}
	if s.WrongStreakPenaltyMinutes < 0 {
		s.WrongStreakPenaltyMinutes = d.WrongStreakPenaltyMinutes
	}
	return s
}

func EffectiveReward(task EarnTask, settings EarnSettings) int {
	if task.RewardMinutes > 0 {
		return task.RewardMinutes
	}
	return NormalizeEarnSettings(settings).DefaultRewardMinutes
}

func TaskKind(task EarnTask) string {
	if task.Kind == EarnKindChoice || len(task.Choices) > 0 {
		return EarnKindChoice
	}
	return EarnKindText
}

func (c EarnChallenge) Public() EarnPublicTask {
	kind := c.Kind
	if kind == "" {
		if len(c.Choices) > 0 {
			kind = EarnKindChoice
		} else {
			kind = EarnKindText
		}
	}
	return EarnPublicTask{
		ID:            c.ID,
		Prompt:        c.Prompt,
		Kind:          kind,
		Choices:       append([]string(nil), c.Choices...),
		RewardMinutes: c.RewardMinutes,
		Source:        c.Source,
		WrongCount:    c.WrongCount,
	}
}

// ResolveEarnSettings fills defaults for legacy stored settings.
func ResolveEarnSettings(s EarnSettings) EarnSettings {
	if s.DefaultRewardMinutes == 0 && s.MaxEarnPerDay == 0 && s.WrongLockSeconds == 0 && s.WrongStreakLimit == 0 {
		return DefaultEarnSettings()
	}
	legacy := s.WrongLockSeconds == 0 && s.WrongStreakLimit == 0
	out := NormalizeEarnSettings(s)
	if legacy {
		// Old JSON lacked lock/streak/generator fields — enable math generator.
		out.MathGeneratorEnabled = true
	} else {
		out.MathGeneratorEnabled = s.MathGeneratorEnabled
	}
	return out
}
