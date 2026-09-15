package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRedeemAvailability_EveningCap(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	now := time.Date(2026, 9, 13, 23, 0, 0, 0, loc)
	av := RedeemAvailabilityAt(now, nil)
	if !av.Allowed || av.MaxMinutes != 60 {
		t.Fatalf("at 23:00 want max 60, got %+v", av)
	}
	if err := ValidateRedeemMinutes(now, 120, nil); !errors.Is(err, ErrRedeemTooLong) {
		t.Fatalf("120 min at 23:00: %v", err)
	}
	if err := ValidateRedeemMinutes(now, 60, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRedeemAvailability_QuietHours(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	now := time.Date(2026, 9, 14, 1, 30, 0, 0, loc)
	av := RedeemAvailabilityAt(now, nil)
	if av.Allowed || av.MaxMinutes != 0 {
		t.Fatalf("at 01:30 want blocked, got %+v", av)
	}
	if err := ValidateRedeemMinutes(now, 5, nil); !errors.Is(err, ErrRedeemCurfew) {
		t.Fatalf("err=%v", err)
	}
}

func TestRedeemAvailability_DaytimeLongOK(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, loc)
	av := RedeemAvailabilityAt(now, nil)
	if !av.Allowed || av.MaxMinutes != 14*60 {
		t.Fatalf("at 10:00 want 840 min, got %+v", av)
	}
	msg := av.Message
	if !strings.Contains(msg, "840") {
		t.Fatalf("message=%q", msg)
	}
}

func TestRedeemAvailability_CustomSchedule(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	// Sunday 2026-09-13
	schedule := DaySchedule{
		"sunday": {{Start: "14:00", End: "16:00"}},
	}
	outside := time.Date(2026, 9, 13, 10, 0, 0, 0, loc)
	av := RedeemAvailabilityAt(outside, schedule)
	if av.Allowed {
		t.Fatalf("10:00 outside window: %+v", av)
	}
	inside := time.Date(2026, 9, 13, 15, 0, 0, 0, loc)
	av = RedeemAvailabilityAt(inside, schedule)
	if !av.Allowed || av.MaxMinutes != 60 {
		t.Fatalf("15:00 want max 60, got %+v", av)
	}
	emptyDay := time.Date(2026, 9, 14, 15, 0, 0, 0, loc) // monday, not in schedule
	av = RedeemAvailabilityAt(emptyDay, schedule)
	if av.Allowed {
		t.Fatalf("monday with no intervals should block: %+v", av)
	}
}

func TestRedeemAvailability_EmptyDayBlocks(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	schedule := DaySchedule{
		"monday":    {{Start: "08:00", End: "00:00"}},
		"tuesday":   {},
		"wednesday": {{Start: "08:00", End: "00:00"}},
		"thursday":  {{Start: "08:00", End: "00:00"}},
		"friday":    {{Start: "08:00", End: "00:00"}},
		"saturday":  {{Start: "08:00", End: "00:00"}},
		"sunday":    {{Start: "08:00", End: "00:00"}},
	}
	// 2026-09-15 is Tuesday
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, loc)
	av := RedeemAvailabilityAt(now, schedule)
	if av.Allowed {
		t.Fatalf("empty tuesday should block: %+v", av)
	}
}
