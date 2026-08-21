package server

import (
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

func TestAggregateDayActivity(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, loc)
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, loc)

	login := time.Date(2026, 8, 21, 10, 0, 0, 0, loc)
	focusChrome := time.Date(2026, 8, 21, 10, 5, 0, 0, loc)
	blurChrome := time.Date(2026, 8, 21, 10, 15, 0, 0, loc)
	logout := time.Date(2026, 8, 21, 12, 0, 0, 0, loc)

	events := []domain.ActivityEvent{
		{Type: domain.EventSessionLogin, Timestamp: login, Username: "kid", SessionID: 1},
		{Type: domain.EventAppOpen, Timestamp: login, AppName: "Chrome", ExePath: "chrome.exe", Username: "kid"},
		{Type: domain.EventAppFocus, Timestamp: focusChrome, AppName: "Chrome", ExePath: "chrome.exe"},
		{Type: domain.EventAppBlur, Timestamp: blurChrome, AppName: "Chrome", ExePath: "chrome.exe", DurationMs: 10 * 60 * 1000},
		{Type: domain.EventAppClose, Timestamp: logout, AppName: "Chrome", ExePath: "chrome.exe", DurationMs: 2 * 60 * 60 * 1000},
		{Type: domain.EventSessionLogout, Timestamp: logout, Username: "kid", SessionID: 1},
	}

	agg := AggregateDayActivity(day, events, now)
	if agg.Date != "2026-08-21" {
		t.Fatalf("date = %s", agg.Date)
	}
	if len(agg.Sessions) != 1 {
		t.Fatalf("sessions = %d", len(agg.Sessions))
	}
	if agg.Sessions[0].DurationMs != 2*60*60*1000 {
		t.Errorf("session duration = %d", agg.Sessions[0].DurationMs)
	}
	if len(agg.Apps) != 1 {
		t.Fatalf("apps = %d", len(agg.Apps))
	}
	if agg.Apps[0].FocusMs != 10*60*1000 {
		t.Errorf("focus = %d", agg.Apps[0].FocusMs)
	}
	if agg.Apps[0].OpenMs != 2*60*60*1000 {
		t.Errorf("open = %d", agg.Apps[0].OpenMs)
	}
	if len(agg.FocusTimeline) != 1 {
		t.Fatalf("timeline = %d", len(agg.FocusTimeline))
	}
}
