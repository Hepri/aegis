package domain

import (
	"fmt"
	"time"
)

const (
	// QuietHoursStartHour is inclusive local hour when PC time must not be bought into.
	QuietHoursStartHour = 0
	// QuietHoursEndHour is exclusive local hour (00:00–08:00).
	QuietHoursEndHour = 8

	EarnLogAnswerCorrect = "answer_correct"
	EarnLogAnswerWrong   = "answer_wrong"
	EarnLogPenalty       = "penalty"
	EarnLogSkip          = "skip"
	EarnLogRedeem        = "redeem"
	EarnLogRefund        = "refund_session"
	EarnLogClearBalance  = "clear_balance"

	TempAccessSourceEarn = "earn"

	MaxEarnLogEntries = 1000
)

// EarnLogEntry is one wallet/challenge operation stored on the client.
type EarnLogEntry struct {
	ID           string    `json:"id"`
	At           time.Time `json:"at"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name,omitempty"`
	Kind         string    `json:"kind"`
	MinutesDelta int       `json:"minutes_delta"`
	BalanceAfter int       `json:"balance_after"`
	TaskID       string    `json:"task_id,omitempty"`
	Subject      string    `json:"subject,omitempty"`
	Prompt       string    `json:"prompt,omitempty"`
	Answer       string    `json:"answer,omitempty"`       // expected / correct
	GivenAnswer  string    `json:"given_answer,omitempty"` // what the child typed/chose
	Detail       string    `json:"detail,omitempty"`
}

// EarnRedeemResult is returned after a successful purchase.
type EarnRedeemResult struct {
	Minutes    int       `json:"minutes"`
	Until      time.Time `json:"until"`
	Balance    int       `json:"balance_minutes"`
	Message    string    `json:"message"`
	MaxAllowed int       `json:"max_allowed_minutes,omitempty"`
}

// EarnRefundResult is returned after ending an earn session early.
type EarnRefundResult struct {
	RefundedMinutes int       `json:"refunded_minutes"`
	Balance         int       `json:"balance_minutes"`
	Message         string    `json:"message"`
	EndedAt         time.Time `json:"ended_at"`
}

// ActiveEarnAccess describes remaining bought (earn) temporary access.
type ActiveEarnAccess struct {
	RemainingMinutes int       `json:"remaining_minutes"`
	Until            time.Time `json:"until"`
}

// RedeemAvailability describes how much time can be bought right now.
type RedeemAvailability struct {
	Allowed     bool      `json:"allowed"`
	MaxMinutes  int       `json:"max_minutes"`
	QuietUntil  time.Time `json:"quiet_until,omitempty"` // access must end by this instant
	Message     string    `json:"message,omitempty"`
}

// InQuietHours reports whether local clock is in [00:00, 08:00).
func InQuietHours(now time.Time) bool {
	h := now.Hour()
	return h >= QuietHoursStartHour && h < QuietHoursEndHour
}

// NextQuietStart is the next local midnight (start of 00:00–08:00 quiet window).
func NextQuietStart(now time.Time) time.Time {
	loc := now.Location()
	return time.Date(now.Year(), now.Month(), now.Day()+1, QuietHoursStartHour, 0, 0, 0, loc)
}

// RedeemAvailabilityAt returns how many minutes may be bought at now.
func RedeemAvailabilityAt(now time.Time) RedeemAvailability {
	if InQuietHours(now) {
		until := time.Date(now.Year(), now.Month(), now.Day(), QuietHoursEndHour, 0, 0, 0, now.Location())
		return RedeemAvailability{
			Allowed:    false,
			MaxMinutes: 0,
			QuietUntil: until,
			Message:    "Сейчас ночь (00:00–08:00) — купить время нельзя",
		}
	}
	quiet := NextQuietStart(now)
	maxMin := int(quiet.Sub(now) / time.Minute)
	if maxMin < 1 {
		return RedeemAvailability{
			Allowed:    false,
			MaxMinutes: 0,
			QuietUntil: quiet,
			Message:    "До полуночи меньше минуты — купить время нельзя",
		}
	}
	return RedeemAvailability{
		Allowed:    true,
		MaxMinutes: maxMin,
		QuietUntil: quiet,
		Message:    fmt.Sprintf("Можно купить не больше %d мин (до 00:00 — ночью ПК закрыт)", maxMin),
	}
}

// ValidateRedeemMinutes checks a purchase request against quiet hours.
func ValidateRedeemMinutes(now time.Time, minutes int) error {
	if minutes <= 0 {
		return ErrInvalidMinutes
	}
	av := RedeemAvailabilityAt(now)
	if !av.Allowed {
		return ErrRedeemCurfew
	}
	if minutes > av.MaxMinutes {
		return fmt.Errorf("%w: максимум %d мин до 00:00", ErrRedeemTooLong, av.MaxMinutes)
	}
	return nil
}
