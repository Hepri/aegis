package client

import (
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/google/uuid"
)

// SessionSnapshot is the observed state of one WTS session.
type SessionSnapshot struct {
	SessionID uint32
	Username  string
	State     SessionState
	// LogonTime is the real Windows session logon time when known; zero if unknown.
	LogonTime time.Time
}

// SessionState mirrors WTS connect states we care about.
type SessionState int

const (
	SessionActive SessionState = iota
	SessionDisconnected
	SessionOther
)

// AppSnapshot is one visible application window process.
type AppSnapshot struct {
	AppName string
	ExePath string
}

// AppWatchState is a full snapshot of open apps + foreground.
type AppWatchState struct {
	Username string
	Apps     []AppSnapshot
	Focused  *AppSnapshot
}

// DiffSessions compares previous and current session maps and emits login/logout/lock/unlock events.
// Map key is SessionID.
func DiffSessions(prev, curr map[uint32]SessionSnapshot, now time.Time) []domain.ActivityEvent {
	var events []domain.ActivityEvent
	for id, c := range curr {
		p, ok := prev[id]
		if !ok {
			if c.State == SessionActive || c.State == SessionDisconnected {
				ts := now
				if !c.LogonTime.IsZero() {
					ts = c.LogonTime
				}
				events = append(events, domain.ActivityEvent{
					ID:        uuid.New().String(),
					Type:      domain.EventSessionLogin,
					Timestamp: ts,
					Username:  c.Username,
					SessionID: id,
				})
				if c.State == SessionDisconnected {
					events = append(events, domain.ActivityEvent{
						ID:        uuid.New().String(),
						Type:      domain.EventSessionLock,
						Timestamp: now,
						Username:  c.Username,
						SessionID: id,
					})
				}
			}
			continue
		}
		if p.State == SessionActive && c.State == SessionDisconnected {
			events = append(events, domain.ActivityEvent{
				ID:        uuid.New().String(),
				Type:      domain.EventSessionLock,
				Timestamp: now,
				Username:  c.Username,
				SessionID: id,
			})
		}
		if p.State == SessionDisconnected && c.State == SessionActive {
			events = append(events, domain.ActivityEvent{
				ID:        uuid.New().String(),
				Type:      domain.EventSessionUnlock,
				Timestamp: now,
				Username:  c.Username,
				SessionID: id,
			})
		}
	}
	for id, p := range prev {
		if _, ok := curr[id]; !ok {
			if p.State == SessionActive || p.State == SessionDisconnected {
				events = append(events, domain.ActivityEvent{
					ID:        uuid.New().String(),
					Type:      domain.EventSessionLogout,
					Timestamp: now,
					Username:  p.Username,
					SessionID: id,
				})
			}
		}
	}
	return events
}

func appKey(a AppSnapshot) string {
	if a.ExePath != "" {
		return a.ExePath
	}
	return a.AppName
}

// DiffApps compares previous and current app watch state.
func DiffApps(prev, curr *AppWatchState, now time.Time, openSince map[string]time.Time, focusSince *time.Time, focusKey string) (
	events []domain.ActivityEvent,
	newOpenSince map[string]time.Time,
	newFocusSince *time.Time,
	newFocusKey string,
) {
	newOpenSince = make(map[string]time.Time)
	if prev == nil {
		prev = &AppWatchState{}
	}
	if curr == nil {
		curr = &AppWatchState{}
	}
	username := curr.Username
	if username == "" {
		username = prev.Username
	}

	prevMap := make(map[string]AppSnapshot)
	for _, a := range prev.Apps {
		prevMap[appKey(a)] = a
	}
	currMap := make(map[string]AppSnapshot)
	for _, a := range curr.Apps {
		currMap[appKey(a)] = a
	}

	for k, a := range currMap {
		if _, ok := prevMap[k]; !ok {
			events = append(events, domain.ActivityEvent{
				ID:        uuid.New().String(),
				Type:      domain.EventAppOpen,
				Timestamp: now,
				Username:  username,
				AppName:   a.AppName,
				ExePath:   a.ExePath,
			})
			newOpenSince[k] = now
		} else if t, ok := openSince[k]; ok {
			newOpenSince[k] = t
		} else {
			newOpenSince[k] = now
		}
	}
	for k, a := range prevMap {
		if _, ok := currMap[k]; !ok {
			dur := int64(0)
			if t, ok := openSince[k]; ok {
				dur = now.Sub(t).Milliseconds()
			}
			events = append(events, domain.ActivityEvent{
				ID:         uuid.New().String(),
				Type:       domain.EventAppClose,
				Timestamp:  now,
				Username:   username,
				AppName:    a.AppName,
				ExePath:    a.ExePath,
				DurationMs: dur,
			})
		}
	}

	var currFocusKey string
	var currFocus *AppSnapshot
	if curr.Focused != nil {
		currFocus = curr.Focused
		currFocusKey = appKey(*curr.Focused)
	}
	prevFocusKey := focusKey

	if currFocusKey != prevFocusKey {
		if prevFocusKey != "" && focusSince != nil {
			dur := now.Sub(*focusSince).Milliseconds()
			var prevApp AppSnapshot
			if p, ok := prevMap[prevFocusKey]; ok {
				prevApp = p
			} else {
				prevApp = AppSnapshot{AppName: prevFocusKey, ExePath: prevFocusKey}
			}
			events = append(events, domain.ActivityEvent{
				ID:         uuid.New().String(),
				Type:       domain.EventAppBlur,
				Timestamp:  now,
				Username:   username,
				AppName:    prevApp.AppName,
				ExePath:    prevApp.ExePath,
				DurationMs: dur,
			})
		}
		if currFocus != nil {
			events = append(events, domain.ActivityEvent{
				ID:        uuid.New().String(),
				Type:      domain.EventAppFocus,
				Timestamp: now,
				Username:  username,
				AppName:   currFocus.AppName,
				ExePath:   currFocus.ExePath,
			})
			t := now
			newFocusSince = &t
			newFocusKey = currFocusKey
		} else {
			newFocusSince = nil
			newFocusKey = ""
		}
	} else {
		newFocusSince = focusSince
		newFocusKey = focusKey
	}
	return events, newOpenSince, newFocusSince, newFocusKey
}
