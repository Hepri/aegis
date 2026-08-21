package server

import (
	"sort"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// AggregateDayActivity builds sessions, app totals, and focus timeline from raw events.
func AggregateDayActivity(day time.Time, events []domain.ActivityEvent, now time.Time) domain.DayActivity {
	dayStr := day.Format("2006-01-02")
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := dayStart.Add(24 * time.Hour)
	if now.After(dayEnd) {
		now = dayEnd
	}
	if now.Before(dayStart) {
		now = dayStart
	}

	sorted := append([]domain.ActivityEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	type openSess struct {
		username string
		login    time.Time
		lockedAt *time.Time
		lockedMs int64
	}
	openSessions := map[uint32]*openSess{}
	var sessions []domain.SessionSummary

	type openApp struct {
		name   string
		exe    string
		opened time.Time
		openMs int64
	}
	appsOpen := map[string]*openApp{} // key: exe|name
	appTotals := map[string]*domain.AppSummary{}

	var focusTimeline []domain.FocusSpan
	var focusStart *time.Time
	var focusApp, focusExe string

	appKey := func(name, exe string) string {
		if exe != "" {
			return exe
		}
		return name
	}

	ensureApp := func(name, exe string) *domain.AppSummary {
		k := appKey(name, exe)
		if a, ok := appTotals[k]; ok {
			return a
		}
		a := &domain.AppSummary{AppName: name, ExePath: exe}
		appTotals[k] = a
		return a
	}

	closeFocus := func(at time.Time) {
		if focusStart == nil {
			return
		}
		end := at
		if end.Before(*focusStart) {
			end = *focusStart
		}
		dur := end.Sub(*focusStart).Milliseconds()
		if dur < 0 {
			dur = 0
		}
		ensureApp(focusApp, focusExe).FocusMs += dur
		focusTimeline = append(focusTimeline, domain.FocusSpan{
			AppName: focusApp,
			ExePath: focusExe,
			Start:   *focusStart,
			End:     end,
		})
		focusStart = nil
	}

	for _, ev := range sorted {
		ts := ev.Timestamp
		switch ev.Type {
		case domain.EventSessionLogin:
			openSessions[ev.SessionID] = &openSess{username: ev.Username, login: ts}
		case domain.EventSessionLock:
			if s := openSessions[ev.SessionID]; s != nil && s.lockedAt == nil {
				t := ts
				s.lockedAt = &t
			}
		case domain.EventSessionUnlock:
			if s := openSessions[ev.SessionID]; s != nil && s.lockedAt != nil {
				s.lockedMs += ts.Sub(*s.lockedAt).Milliseconds()
				s.lockedAt = nil
			}
		case domain.EventSessionLogout:
			s := openSessions[ev.SessionID]
			if s == nil {
				login := ts
				logout := ts
				sessions = append(sessions, domain.SessionSummary{
					Username:   ev.Username,
					Login:      login,
					Logout:     &logout,
					DurationMs: 0,
				})
				continue
			}
			if s.lockedAt != nil {
				s.lockedMs += ts.Sub(*s.lockedAt).Milliseconds()
				s.lockedAt = nil
			}
			logout := ts
			sessions = append(sessions, domain.SessionSummary{
				Username:   s.username,
				Login:      s.login,
				Logout:     &logout,
				DurationMs: ts.Sub(s.login).Milliseconds(),
				LockedMs:   s.lockedMs,
			})
			delete(openSessions, ev.SessionID)

		case domain.EventAppOpen:
			k := appKey(ev.AppName, ev.ExePath)
			appsOpen[k] = &openApp{name: ev.AppName, exe: ev.ExePath, opened: ts}
			ensureApp(ev.AppName, ev.ExePath)
		case domain.EventAppClose:
			k := appKey(ev.AppName, ev.ExePath)
			a := ensureApp(ev.AppName, ev.ExePath)
			if o, ok := appsOpen[k]; ok {
				dur := ev.DurationMs
				if dur <= 0 {
					dur = ts.Sub(o.opened).Milliseconds()
				}
				a.OpenMs += dur
				delete(appsOpen, k)
			} else if ev.DurationMs > 0 {
				a.OpenMs += ev.DurationMs
			}
			if focusStart != nil && appKey(focusApp, focusExe) == k {
				closeFocus(ts)
			}
		case domain.EventAppFocus:
			closeFocus(ts)
			t := ts
			focusStart = &t
			focusApp = ev.AppName
			focusExe = ev.ExePath
			ensureApp(ev.AppName, ev.ExePath)
		case domain.EventAppBlur:
			if focusStart != nil {
				if ev.DurationMs > 0 {
					ensureApp(focusApp, focusExe).FocusMs += ev.DurationMs
					end := focusStart.Add(time.Duration(ev.DurationMs) * time.Millisecond)
					focusTimeline = append(focusTimeline, domain.FocusSpan{
						AppName: focusApp,
						ExePath: focusExe,
						Start:   *focusStart,
						End:     end,
					})
					focusStart = nil
				} else {
					closeFocus(ts)
				}
			}
		}
	}

	// Close still-open sessions/apps at end of day / now
	for _, s := range openSessions {
		if s.lockedAt != nil {
			s.lockedMs += now.Sub(*s.lockedAt).Milliseconds()
		}
		sessions = append(sessions, domain.SessionSummary{
			Username:   s.username,
			Login:      s.login,
			DurationMs: now.Sub(s.login).Milliseconds(),
			LockedMs:   s.lockedMs,
		})
	}
	for _, o := range appsOpen {
		a := ensureApp(o.name, o.exe)
		a.OpenMs += now.Sub(o.opened).Milliseconds()
	}
	closeFocus(now)

	apps := make([]domain.AppSummary, 0, len(appTotals))
	for _, a := range appTotals {
		apps = append(apps, *a)
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].FocusMs != apps[j].FocusMs {
			return apps[i].FocusMs > apps[j].FocusMs
		}
		return apps[i].OpenMs > apps[j].OpenMs
	})
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Login.Before(sessions[j].Login)
	})
	sort.Slice(focusTimeline, func(i, j int) bool {
		return focusTimeline[i].Start.Before(focusTimeline[j].Start)
	})

	return domain.DayActivity{
		Date:          dayStr,
		Sessions:      sessions,
		Apps:          apps,
		FocusTimeline: focusTimeline,
	}
}
