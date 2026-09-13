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
	av := RedeemAvailabilityAt(now)
	if !av.Allowed || av.MaxMinutes != 60 {
		t.Fatalf("at 23:00 want max 60, got %+v", av)
	}
	if err := ValidateRedeemMinutes(now, 120); !errors.Is(err, ErrRedeemTooLong) {
		t.Fatalf("120 min at 23:00: %v", err)
	}
	if err := ValidateRedeemMinutes(now, 60); err != nil {
		t.Fatal(err)
	}
}

func TestRedeemAvailability_QuietHours(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	now := time.Date(2026, 9, 14, 1, 30, 0, 0, loc)
	av := RedeemAvailabilityAt(now)
	if av.Allowed || av.MaxMinutes != 0 {
		t.Fatalf("at 01:30 want blocked, got %+v", av)
	}
	if err := ValidateRedeemMinutes(now, 5); !errors.Is(err, ErrRedeemCurfew) {
		t.Fatalf("err=%v", err)
	}
}

func TestRedeemAvailability_DaytimeLongOK(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, loc)
	av := RedeemAvailabilityAt(now)
	if !av.Allowed || av.MaxMinutes != 14*60 {
		t.Fatalf("at 10:00 want 840 min, got %+v", av)
	}
	msg := av.Message
	if !strings.Contains(msg, "840") && !strings.Contains(msg, "00:00") {
		t.Fatalf("message=%q", msg)
	}
}
