package client

import (
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

func TestDiffSessions_LoginUsesLogonTime(t *testing.T) {
	now := time.Now()
	logon := now.Add(-3 * time.Minute)
	prev := map[uint32]SessionSnapshot{}
	curr := map[uint32]SessionSnapshot{
		1: {SessionID: 1, Username: "kid", State: SessionActive, LogonTime: logon},
	}
	ev := DiffSessions(prev, curr, now)
	if len(ev) != 1 || ev[0].Type != domain.EventSessionLogin {
		t.Fatalf("login: %+v", ev)
	}
	if !ev[0].Timestamp.Equal(logon) {
		t.Fatalf("timestamp = %v, want logon %v", ev[0].Timestamp, logon)
	}
}

func TestDiffSessions_LoginLockUnlockLogout(t *testing.T) {
	now := time.Now()
	prev := map[uint32]SessionSnapshot{}
	curr := map[uint32]SessionSnapshot{
		1: {SessionID: 1, Username: "kid", State: SessionActive},
	}
	ev := DiffSessions(prev, curr, now)
	if len(ev) != 1 || ev[0].Type != domain.EventSessionLogin {
		t.Fatalf("login: %+v", ev)
	}

	prev = curr
	curr = map[uint32]SessionSnapshot{
		1: {SessionID: 1, Username: "kid", State: SessionDisconnected},
	}
	ev = DiffSessions(prev, curr, now)
	if len(ev) != 1 || ev[0].Type != domain.EventSessionLock {
		t.Fatalf("lock: %+v", ev)
	}

	prev = curr
	curr = map[uint32]SessionSnapshot{
		1: {SessionID: 1, Username: "kid", State: SessionActive},
	}
	ev = DiffSessions(prev, curr, now)
	if len(ev) != 1 || ev[0].Type != domain.EventSessionUnlock {
		t.Fatalf("unlock: %+v", ev)
	}

	prev = curr
	curr = map[uint32]SessionSnapshot{}
	ev = DiffSessions(prev, curr, now)
	if len(ev) != 1 || ev[0].Type != domain.EventSessionLogout {
		t.Fatalf("logout: %+v", ev)
	}
}

func TestDiffApps_OpenFocusClose(t *testing.T) {
	now := time.Now()
	openSince := map[string]time.Time{}
	var focusSince *time.Time
	focusKey := ""

	curr := &AppWatchState{
		Username: "kid",
		Apps:     []AppSnapshot{{AppName: "Chrome", ExePath: `C:\chrome.exe`}},
		Focused:  &AppSnapshot{AppName: "Chrome", ExePath: `C:\chrome.exe`},
	}
	ev, openSince, focusSince, focusKey := DiffApps(nil, curr, now, openSince, focusSince, focusKey, 1)
	types := map[string]int{}
	for _, e := range ev {
		types[e.Type]++
	}
	if types[domain.EventAppOpen] != 1 || types[domain.EventAppFocus] != 1 {
		t.Fatalf("open+focus: %+v", ev)
	}

	later := now.Add(5 * time.Minute)
	next := &AppWatchState{Username: "kid", Apps: nil, Focused: nil}
	ev, _, _, _ = DiffApps(curr, next, later, openSince, focusSince, focusKey, 1)
	types = map[string]int{}
	for _, e := range ev {
		types[e.Type]++
		if e.Type == domain.EventAppBlur && e.DurationMs != 5*60*1000 {
			t.Errorf("blur duration = %d", e.DurationMs)
		}
		if e.Type == domain.EventAppClose && e.DurationMs != 5*60*1000 {
			t.Errorf("close duration = %d", e.DurationMs)
		}
	}
	if types[domain.EventAppClose] != 1 || types[domain.EventAppBlur] != 1 {
		t.Fatalf("close+blur: %+v", ev)
	}
}
