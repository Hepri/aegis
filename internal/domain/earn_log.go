package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	EarnLogAnswerCorrect = "answer_correct"
	EarnLogAnswerWrong   = "answer_wrong"
	EarnLogPenalty       = "penalty"
	EarnLogSkip          = "skip"
	EarnLogRedeem        = "redeem"
	EarnLogRefund        = "refund_session"
	EarnLogClearBalance  = "clear_balance"
	EarnLogAdjustBalance = "adjust_balance"

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
	QuietUntil  time.Time `json:"quiet_until,omitempty"` // access must end by this instant (window end)
	Message     string    `json:"message,omitempty"`
}

// DefaultRedeemSchedule is Mon–Sun 08:00–00:00 (buy until midnight).
func DefaultRedeemSchedule() DaySchedule {
	iv := []TimeInterval{{Start: "08:00", End: "00:00"}}
	out := DaySchedule{}
	for _, d := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		out[d] = append([]TimeInterval(nil), iv...)
	}
	return out
}

// RedeemScheduleHasIntervals reports whether any day has at least one window.
func RedeemScheduleHasIntervals(s DaySchedule) bool {
	for _, ivs := range s {
		if len(ivs) > 0 {
			return true
		}
	}
	return false
}

// ResolveRedeemSchedule returns s when it has windows; otherwise the default 08:00–00:00 schedule.
func ResolveRedeemSchedule(s DaySchedule) DaySchedule {
	if RedeemScheduleHasIntervals(s) {
		return s
	}
	return DefaultRedeemSchedule()
}

func redeemDayKey(t time.Time) string {
	return strings.ToLower(t.Weekday().String())
}

func findRedeemWindow(now time.Time, schedule DaySchedule) (start, end time.Time, ok bool) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// Check today and yesterday (overnight windows that spill into today).
	for _, dayOffset := range []int{0, -1} {
		d := day.AddDate(0, 0, dayOffset)
		for _, iv := range schedule[redeemDayKey(d)] {
			st, en, err := parseDayInterval(d, iv.Start, iv.End)
			if err != nil {
				continue
			}
			if !now.Before(st) && now.Before(en) {
				return st, en, true
			}
		}
	}
	return time.Time{}, time.Time{}, false
}

func nextRedeemWindowStart(now time.Time, schedule DaySchedule) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var best time.Time
	for dayOffset := 0; dayOffset < 8; dayOffset++ {
		d := day.AddDate(0, 0, dayOffset)
		for _, iv := range schedule[redeemDayKey(d)] {
			st, en, err := parseDayInterval(d, iv.Start, iv.End)
			if err != nil || !en.After(st) {
				continue
			}
			if st.After(now) && (best.IsZero() || st.Before(best)) {
				best = st
			}
		}
	}
	return best
}

// InQuietHours reports whether default redeem schedule blocks buying (legacy helper).
func InQuietHours(now time.Time) bool {
	av := RedeemAvailabilityAt(now, nil)
	return !av.Allowed
}

// RedeemAvailabilityAt returns how many minutes may be bought at now under redeem_schedule.
// A nil/empty schedule uses DefaultRedeemSchedule (08:00–00:00 every day).
// Empty day (no intervals) means buying is not allowed that day.
func RedeemAvailabilityAt(now time.Time, schedule DaySchedule) RedeemAvailability {
	schedule = ResolveRedeemSchedule(schedule)
	if _, end, ok := findRedeemWindow(now, schedule); ok {
		maxMin := int(end.Sub(now) / time.Minute)
		if maxMin < 1 {
			return RedeemAvailability{
				Allowed:    false,
				MaxMinutes: 0,
				QuietUntil: end,
				Message:    "До конца окна покупки меньше минуты — купить время нельзя",
			}
		}
		return RedeemAvailability{
			Allowed:    true,
			MaxMinutes: maxMin,
			QuietUntil: end,
			Message: fmt.Sprintf("Можно купить не больше %d мин (до %s)",
				maxMin, end.Format("15:04")),
		}
	}
	next := nextRedeemWindowStart(now, schedule)
	msg := "Сейчас нельзя купить время"
	if !next.IsZero() {
		msg = fmt.Sprintf("Сейчас нельзя купить время (следующее окно с %s)", next.Format("15:04"))
	}
	return RedeemAvailability{
		Allowed:    false,
		MaxMinutes: 0,
		QuietUntil: next,
		Message:    msg,
	}
}

// ValidateRedeemMinutes checks a purchase request against the redeem schedule.
func ValidateRedeemMinutes(now time.Time, minutes int, schedule DaySchedule) error {
	if minutes <= 0 {
		return ErrInvalidMinutes
	}
	av := RedeemAvailabilityAt(now, schedule)
	if !av.Allowed {
		return ErrRedeemCurfew
	}
	if minutes > av.MaxMinutes {
		return fmt.Errorf("%w: максимум %d мин до %s", ErrRedeemTooLong, av.MaxMinutes, av.QuietUntil.Format("15:04"))
	}
	return nil
}
