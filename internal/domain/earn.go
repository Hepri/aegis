package domain

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var (
	ErrClientNotFound      = errors.New("клиент не найден")
	ErrUserNotFound        = errors.New("пользователь не найден")
	ErrTaskNotFound        = errors.New("задача не найдена")
	ErrTaskAlreadySolved   = errors.New("задача уже решена")
	ErrDailyLimit          = errors.New("дневной лимит траты исчерпан")
	ErrInsufficientBalance = errors.New("недостаточно минут на балансе")
	ErrInvalidMinutes      = errors.New("число минут должно быть больше нуля")
	ErrEarnLocked          = errors.New("пауза после ошибки")
	ErrRedeemDisabled = errors.New("покупка времени отключена")
	ErrRedeemCurfew   = errors.New("сейчас нельзя купить время")
	ErrRedeemTooLong  = errors.New("покупка выходит за окно покупки")
	ErrNoEarnSession  = errors.New("нет активной купленной сессии для возврата")
	ErrBalanceFull    = errors.New("достигнут лимит накопленного времени")
	ErrBuiltinTask    = errors.New("встроенный вопрос нельзя менять")
	ErrInvalidTask    = errors.New("некорректный вопрос")
)

const (
	EarnKindText     = "text"
	EarnKindChoice   = "choice"
	EarnSourceBank   = "bank"
	EarnSourceGen    = "generated"
	EarnSourceCustom = "custom"
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
	Subject       string    `json:"subject,omitempty"`
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
	Subject       string   `json:"subject,omitempty"`
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
	DefaultRewardMinutes int `json:"default_reward_minutes"`
	// MaxEarnPerDay is the daily redeem/spend cap («купить время»), not earn-from-tasks.
	// JSON key kept for existing admin settings.
	MaxEarnPerDay int `json:"max_earn_per_day"`
	// MaxBalanceMinutes is the wallet cap («максимум накопленного времени»).
	MaxBalanceMinutes         int         `json:"max_balance_minutes"`
	WrongLockSeconds          int         `json:"wrong_lock_seconds"`
	WrongStreakLimit          int         `json:"wrong_streak_limit"`           // N wrong answers → penalty + new question
	WrongStreakPenaltyMinutes int         `json:"wrong_streak_penalty_minutes"` // minutes removed after N wrongs
	MathGeneratorEnabled      bool        `json:"math_generator_enabled"`
	DisableRedeem             bool        `json:"disable_redeem"`               // true → hide/block «купить время»
	DisabledSubjects          []string    `json:"disabled_subjects,omitempty"` // bank subjects to skip (see EarnBankSubjects)
	// RepeatableSubjects: solved tasks from these sections stay in rotation.
	// nil/omitted → default [мораль, таблица умножения]; explicit empty list disables all repeats.
	RepeatableSubjects []string `json:"repeatable_subjects"`
	// SubjectRewardMinutes overrides DefaultRewardMinutes per section (subject → minutes).
	// nil → default {таблица умножения: 1}.
	SubjectRewardMinutes map[string]int `json:"subject_reward_minutes,omitempty"`
	// SubjectWrongPenaltyMinutes: minutes removed on each wrong answer for that section.
	// nil → default {таблица умножения: 5}; explicit empty disables per-section wrong penalties.
	SubjectWrongPenaltyMinutes map[string]int `json:"subject_wrong_penalty_minutes,omitempty"`
	// RedeemSchedule: when «купить время» is allowed. Empty → default Mon–Sun 08:00–00:00.
	// A day with no intervals cannot buy that day.
	RedeemSchedule DaySchedule `json:"redeem_schedule,omitempty"`
}

// EarnDayStats tracks daily earn/spend counters for one user.
type EarnDayStats struct {
	Date          string `json:"date"`
	EarnedMinutes int    `json:"earned_minutes"` // minutes credited from correct answers (informational)
	SpentMinutes  int    `json:"spent_minutes"`  // minutes redeemed («купить время») today
	SolvedCount   int    `json:"solved_count"`
}

func DefaultEarnSettings() EarnSettings {
	return EarnSettings{
		DefaultRewardMinutes:      1,
		MaxEarnPerDay:             120, // daily spend/redeem cap
		MaxBalanceMinutes:         300, // wallet cap
		WrongLockSeconds:          15,
		WrongStreakLimit:          3,
		WrongStreakPenaltyMinutes: 1,
		MathGeneratorEnabled:       true,
		RepeatableSubjects:         []string{SubjectMoral, SubjectMultiply},
		SubjectRewardMinutes:       map[string]int{SubjectMultiply: 1},
		SubjectWrongPenaltyMinutes: map[string]int{SubjectMultiply: 5},
		RedeemSchedule:             DefaultRedeemSchedule(),
	}
}

// DailySpendLimitError explains a blocked redeem with today's spent total.
func DailySpendLimitError(spent, limit, requested int) error {
	return fmt.Errorf("%w: уже потрачено %d из %d мин (запрос %d мин)", ErrDailyLimit, spent, limit, requested)
}

// CreditTowardBalanceCap adds minutes without exceeding maxBalance.
// Returns the new balance and how many minutes were actually credited.
func CreditTowardBalanceCap(balance, add, maxBalance int) (newBalance, credited int) {
	if add <= 0 {
		return balance, 0
	}
	if maxBalance <= 0 {
		return balance + add, add
	}
	if balance >= maxBalance {
		return balance, 0
	}
	room := maxBalance - balance
	if add > room {
		add = room
	}
	return balance + add, add
}

func NormalizeEarnSettings(s EarnSettings) EarnSettings {
	d := DefaultEarnSettings()
	if s.DefaultRewardMinutes <= 0 {
		s.DefaultRewardMinutes = d.DefaultRewardMinutes
	}
	if s.MaxEarnPerDay <= 0 {
		s.MaxEarnPerDay = d.MaxEarnPerDay
	}
	if s.MaxBalanceMinutes <= 0 {
		s.MaxBalanceMinutes = d.MaxBalanceMinutes
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
	s.DisabledSubjects = NormalizeDisabledSubjects(s.DisabledSubjects)
	s.RepeatableSubjects = ResolveRepeatableSubjects(s.RepeatableSubjects)
	s.SubjectRewardMinutes = ResolveSubjectRewardMinutes(s.SubjectRewardMinutes)
	s.SubjectWrongPenaltyMinutes = ResolveSubjectWrongPenaltyMinutes(s.SubjectWrongPenaltyMinutes)
	s.RedeemSchedule = ResolveRedeemSchedule(s.RedeemSchedule)
	return s
}

// NormalizeDisabledSubjects keeps unique known bank subjects only.
func NormalizeDisabledSubjects(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if !IsEarnBankSubject(s) || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ResolveRepeatableSubjects: nil → default [мораль]; empty slice stays empty (all one-shot).
// Allows any non-empty subject name (builtin or custom section).
func ResolveRepeatableSubjects(in []string) []string {
	if in == nil {
		return []string{SubjectMoral, SubjectMultiply}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// SubjectRepeats reports whether solved tasks of this section stay in the draw pool.
func (s EarnSettings) SubjectRepeats(subject string) bool {
	subj := strings.TrimSpace(SubjectLabel(subject))
	for _, d := range ResolveRepeatableSubjects(s.RepeatableSubjects) {
		if d == subj {
			return true
		}
	}
	return false
}

// NormalizeSubjectRewardMinutes keeps positive per-section rewards (1..120).
func NormalizeSubjectRewardMinutes(in map[string]int) map[string]int {
	if len(in) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(in))
	for raw, mins := range in {
		subj := strings.TrimSpace(SubjectLabel(raw))
		if subj == "" || mins <= 0 {
			continue
		}
		if mins > 120 {
			mins = 120
		}
		out[subj] = mins
	}
	return out
}

// ResolveSubjectRewardMinutes: nil → default {таблица умножения: 1}; empty stays empty.
func ResolveSubjectRewardMinutes(in map[string]int) map[string]int {
	if in == nil {
		return map[string]int{SubjectMultiply: 1}
	}
	return NormalizeSubjectRewardMinutes(in)
}

// ResolveSubjectWrongPenaltyMinutes: nil → default {таблица умножения: 5}; empty stays empty.
// Zero is allowed (no penalty for that section).
func ResolveSubjectWrongPenaltyMinutes(in map[string]int) map[string]int {
	if in == nil {
		return map[string]int{SubjectMultiply: 5}
	}
	out := make(map[string]int, len(in))
	for raw, mins := range in {
		subj := strings.TrimSpace(SubjectLabel(raw))
		if subj == "" || mins < 0 {
			continue
		}
		if mins > 60 {
			mins = 60
		}
		out[subj] = mins
	}
	return out
}

// RewardForSubject returns minutes credited for a correct answer in this section.
func (s EarnSettings) RewardForSubject(subject string) int {
	s = NormalizeEarnSettings(s)
	subj := strings.TrimSpace(SubjectLabel(subject))
	if mins, ok := s.SubjectRewardMinutes[subj]; ok && mins > 0 {
		return mins
	}
	return s.DefaultRewardMinutes
}

// WrongPenaltyForSubject returns minutes deducted for a wrong answer in this section.
// 0 means no per-answer subject penalty (global streak penalty may still apply).
func (s EarnSettings) WrongPenaltyForSubject(subject string) int {
	s = NormalizeEarnSettings(s)
	subj := strings.TrimSpace(SubjectLabel(subject))
	if mins, ok := s.SubjectWrongPenaltyMinutes[subj]; ok {
		return mins
	}
	return 0
}

// HasSubjectWrongPenalty reports whether this section uses per-wrong penalties.
func (s EarnSettings) HasSubjectWrongPenalty(subject string) bool {
	_, ok := ResolveSubjectWrongPenaltyMinutes(s.SubjectWrongPenaltyMinutes)[strings.TrimSpace(SubjectLabel(subject))]
	return ok
}

// SubjectBankEnabled reports whether curated bank tasks of this subject may be issued.
// Unknown / empty subjects (custom tasks) stay enabled.
func (s EarnSettings) SubjectBankEnabled(subject string) bool {
	subj := strings.TrimSpace(subject)
	if subj == "" || !IsEarnBankSubject(subj) {
		return true
	}
	for _, d := range s.DisabledSubjects {
		if d == subj {
			return false
		}
	}
	return true
}

// ChallengeAllowed reports whether an active challenge may stay sticky under current settings.
func (s EarnSettings) ChallengeAllowed(ch EarnChallenge) bool {
	if ch.ID == "" {
		return false
	}
	if ch.Source == EarnSourceGen {
		return s.MathGeneratorEnabled
	}
	return s.SubjectBankEnabled(ch.Subject)
}

func EffectiveReward(task EarnTask, settings EarnSettings) int {
	_ = task.RewardMinutes // per-task bank values are ignored; admin section/default wins
	return settings.RewardForSubject(task.Subject)
}

func TaskKind(task EarnTask) string {
	if task.Kind == EarnKindChoice || len(task.Choices) > 0 {
		return EarnKindChoice
	}
	return EarnKindText
}

// NormalizeCustomEarnTask cleans and validates an admin-authored task.
func NormalizeCustomEarnTask(t EarnTask) (EarnTask, error) {
	t.Prompt = strings.TrimSpace(t.Prompt)
	t.Answer = strings.TrimSpace(t.Answer)
	t.Subject = strings.TrimSpace(t.Subject)
	t.ID = strings.TrimSpace(t.ID)
	if t.Prompt == "" || t.Answer == "" || t.Subject == "" {
		return EarnTask{}, fmt.Errorf("%w: нужны раздел, вопрос и ответ", ErrInvalidTask)
	}
	if len(t.Prompt) > 2000 || len(t.Answer) > 500 || len(t.Subject) > 80 {
		return EarnTask{}, fmt.Errorf("%w: слишком длинный текст", ErrInvalidTask)
	}
	seen := map[string]bool{}
	var choices []string
	for _, c := range t.Choices {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		choices = append(choices, c)
	}
	t.Choices = choices
	if len(choices) > 0 {
		t.Kind = EarnKindChoice
		if !seen[t.Answer] {
			t.Choices = append([]string{t.Answer}, t.Choices...)
		}
	} else {
		t.Kind = EarnKindText
		t.Choices = nil
	}
	if t.RewardMinutes <= 0 {
		t.RewardMinutes = 1
	}
	if t.RewardMinutes > 120 {
		t.RewardMinutes = 120
	}
	t.Enabled = true
	return t, nil
}

// IsBuiltinEarnTaskID reports whether id belongs to the shipped bank (not editable).
func IsBuiltinEarnTaskID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	builtinEarnIDsOnce.Do(func() {
		builtinEarnIDs = map[string]bool{}
		for _, t := range BuiltInEarnBank() {
			builtinEarnIDs[t.ID] = true
		}
	})
	return builtinEarnIDs[id]
}

var (
	builtinEarnIDsOnce sync.Once
	builtinEarnIDs     map[string]bool
)

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
		Subject:       SubjectLabel(c.Subject),
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
	out.DisableRedeem = s.DisableRedeem
	out.DisabledSubjects = NormalizeDisabledSubjects(s.DisabledSubjects)
	out.RepeatableSubjects = ResolveRepeatableSubjects(s.RepeatableSubjects)
	out.SubjectRewardMinutes = ResolveSubjectRewardMinutes(s.SubjectRewardMinutes)
	out.SubjectWrongPenaltyMinutes = ResolveSubjectWrongPenaltyMinutes(s.SubjectWrongPenaltyMinutes)
	out.RedeemSchedule = ResolveRedeemSchedule(s.RedeemSchedule)
	return out
}

// ShuffleStrings returns a shuffled copy of in.
func ShuffleStrings(rng *rand.Rand, in []string) []string {
	out := append([]string(nil), in...)
	if rng == nil || len(out) < 2 {
		return out
	}
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
