package server

import (
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

func TestAggregateDayActivity_AppsNestedInSessions(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, loc)
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, loc)

	login := time.Date(2026, 8, 21, 10, 0, 0, 0, loc)
	focusChrome := time.Date(2026, 8, 21, 10, 5, 0, 0, loc)
	blurChrome := time.Date(2026, 8, 21, 10, 15, 0, 0, loc)
	logout := time.Date(2026, 8, 21, 12, 0, 0, 0, loc)

	events := []domain.ActivityEvent{
		{Type: domain.EventSessionLogin, Timestamp: login, Username: "kid", SessionID: 1},
		{Type: domain.EventAppOpen, Timestamp: login, AppName: "Chrome", ExePath: "chrome.exe", Username: "kid", SessionID: 1},
		{Type: domain.EventAppFocus, Timestamp: focusChrome, AppName: "Chrome", ExePath: "chrome.exe", Username: "kid", SessionID: 1},
		{Type: domain.EventAppBlur, Timestamp: blurChrome, AppName: "Chrome", ExePath: "chrome.exe", DurationMs: 10 * 60 * 1000, Username: "kid", SessionID: 1},
		{Type: domain.EventAppClose, Timestamp: logout, AppName: "Chrome", ExePath: "chrome.exe", DurationMs: 2 * 60 * 60 * 1000, Username: "kid", SessionID: 1},
		{Type: domain.EventSessionLogout, Timestamp: logout, Username: "kid", SessionID: 1},
	}

	agg := AggregateDayActivity(day, events, now)
	if agg.Date != "2026-08-21" {
		t.Fatalf("date = %s", agg.Date)
	}
	if len(agg.Sessions) != 1 {
		t.Fatalf("sessions = %d", len(agg.Sessions))
	}
	s := agg.Sessions[0]
	if s.DurationMs != 2*60*60*1000 {
		t.Errorf("session duration = %d", s.DurationMs)
	}
	if len(s.Apps) != 1 || s.Apps[0].AppName != "Chrome" {
		t.Fatalf("apps = %+v", s.Apps)
	}
	if s.Apps[0].FocusMs != 10*60*1000 {
		t.Errorf("focus = %d", s.Apps[0].FocusMs)
	}
	if s.Apps[0].OpenMs != 2*60*60*1000 {
		t.Errorf("open = %d", s.Apps[0].OpenMs)
	}
}

func TestAggregateDayActivity_AttributeByUsernameTime(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, loc)
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, loc)
	login := time.Date(2026, 8, 21, 10, 0, 0, 0, loc)
	appAt := time.Date(2026, 8, 21, 10, 30, 0, 0, loc)

	events := []domain.ActivityEvent{
		{Type: domain.EventSessionLogin, Timestamp: login, Username: "kid", SessionID: 7},
		{Type: domain.EventAppFocus, Timestamp: appAt, AppName: "Game", ExePath: "game.exe", Username: "kid"},
	}
	agg := AggregateDayActivity(day, events, now)
	if len(agg.Sessions) != 1 {
		t.Fatalf("sessions=%d", len(agg.Sessions))
	}
	if len(agg.Sessions[0].Apps) != 1 || agg.Sessions[0].Apps[0].AppName != "Game" {
		t.Fatalf("nested apps=%+v", agg.Sessions[0].Apps)
	}
}

func TestAggregateDayActivity_ClosesGhostOnNewLoginSameUser(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, loc)
	now := time.Date(2026, 8, 21, 21, 10, 0, 0, loc)
	ghostLogin := time.Date(2026, 8, 21, 20, 51, 0, 0, loc)
	realLogin := time.Date(2026, 8, 21, 20, 59, 0, 0, loc)
	realLogout := time.Date(2026, 8, 21, 21, 0, 0, 0, loc)

	events := []domain.ActivityEvent{
		{Type: domain.EventSessionLogin, Timestamp: ghostLogin, Username: "sasha", SessionID: 1},
		{Type: domain.EventAppOpen, Timestamp: ghostLogin, Username: "admin", SessionID: 1, AppName: "Steam"},
		{Type: domain.EventSessionLogin, Timestamp: realLogin, Username: "sasha", SessionID: 3},
		{Type: domain.EventSessionLogout, Timestamp: realLogout, Username: "sasha", SessionID: 3},
	}
	agg := AggregateDayActivity(day, events, now)
	if len(agg.Sessions) != 2 {
		t.Fatalf("sessions=%d %+v", len(agg.Sessions), agg.Sessions)
	}
	for _, s := range agg.Sessions {
		if s.Username == "sasha" && s.SessionID == 1 {
			if s.Logout == nil {
				t.Fatalf("ghost session still open: %+v", s)
			}
			if len(s.Apps) != 0 {
				t.Fatalf("admin apps should not stick to sasha ghost: %+v", s.Apps)
			}
		}
		if s.SessionID == 3 && s.Logout == nil {
			t.Fatalf("real session should be closed: %+v", s)
		}
	}
	open := 0
	for _, s := range agg.Sessions {
		if s.Logout == nil {
			open++
		}
	}
	if open != 0 {
		t.Fatalf("expected no open sessions, open=%d", open)
	}
}

func TestAggregateDayActivity_IgnoresDuplicateLoginSameSession(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, loc)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, loc)
	login := time.Date(2026, 8, 21, 10, 0, 0, 0, loc)
	dup := time.Date(2026, 8, 21, 11, 0, 0, 0, loc)
	events := []domain.ActivityEvent{
		{Type: domain.EventSessionLogin, Timestamp: login, Username: "kid", SessionID: 2},
		{Type: domain.EventSessionLogin, Timestamp: dup, Username: "kid", SessionID: 2},
	}
	agg := AggregateDayActivity(day, events, now)
	if len(agg.Sessions) != 1 {
		t.Fatalf("sessions=%d %+v", len(agg.Sessions), agg.Sessions)
	}
	if !agg.Sessions[0].Login.Equal(login) || agg.Sessions[0].Logout != nil {
		t.Fatalf("want single open session from first login: %+v", agg.Sessions[0])
	}
}

	loc := time.UTC
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, loc)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, loc)
	login := time.Date(2026, 8, 21, 10, 0, 0, 0, loc)
	lock := time.Date(2026, 8, 21, 11, 0, 0, 0, loc)
	events := []domain.ActivityEvent{
		{Type: domain.EventSessionLogin, Timestamp: login, Username: "kid", SessionID: 1},
		{Type: domain.EventSessionLock, Timestamp: lock, Username: "kid", SessionID: 1},
	}
	agg := AggregateDayActivity(day, events, now)
	if len(agg.Sessions) != 1 || !agg.Sessions[0].LockedNow {
		t.Fatalf("want locked_now session, got %+v", agg.Sessions)
	}
}
