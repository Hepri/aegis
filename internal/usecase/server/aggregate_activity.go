package server

import (
	"sort"
	"strings"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

type sessAcc struct {
	sessionID uint32
	username  string
	segStart  time.Time // start of current unlocked usage; zero while locked
	lockedAt  *time.Time
	appsOpen  map[string]*openApp
	appTotals map[string]*domain.AppSummary
	focusStart *time.Time
	focusApp   string
	focusExe   string
}

type openApp struct {
	name   string
	exe    string
	opened time.Time
}

func appKey(name, exe string) string {
	if exe != "" {
		return exe
	}
	return name
}

func (s *sessAcc) ensureApp(name, exe string) *domain.AppSummary {
	k := appKey(name, exe)
	if a, ok := s.appTotals[k]; ok {
		return a
	}
	a := &domain.AppSummary{AppName: name, ExePath: exe}
	s.appTotals[k] = a
	return a
}

func (s *sessAcc) closeFocus(at time.Time) {
	if s.focusStart == nil {
		return
	}
	end := at
	if end.Before(*s.focusStart) {
		end = *s.focusStart
	}
	dur := end.Sub(*s.focusStart).Milliseconds()
	if dur < 0 {
		dur = 0
	}
	s.ensureApp(s.focusApp, s.focusExe).FocusMs += dur
	s.focusStart = nil
}

func (s *sessAcc) resetApps() {
	s.appsOpen = map[string]*openApp{}
	s.appTotals = map[string]*domain.AppSummary{}
	s.focusStart = nil
	s.focusApp = ""
	s.focusExe = ""
}

func (s *sessAcc) unlocked() bool {
	return !s.segStart.IsZero() && s.lockedAt == nil
}

func (s *sessAcc) takeAppSummaries(end time.Time) []domain.AppSummary {
	for _, o := range s.appsOpen {
		s.ensureApp(o.name, o.exe).OpenMs += end.Sub(o.opened).Milliseconds()
	}
	s.appsOpen = map[string]*openApp{}
	s.closeFocus(end)

	apps := make([]domain.AppSummary, 0, len(s.appTotals))
	for _, a := range s.appTotals {
		apps = append(apps, *a)
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].FocusMs != apps[j].FocusMs {
			return apps[i].FocusMs > apps[j].FocusMs
		}
		return apps[i].OpenMs > apps[j].OpenMs
	})
	s.appTotals = map[string]*domain.AppSummary{}
	return apps
}

// flushUnlocked closes the current unlocked usage segment (duration excludes lock screen).
func (s *sessAcc) flushUnlocked(at time.Time, stillOpen bool) *domain.SessionSummary {
	if s.segStart.IsZero() {
		return nil
	}
	end := at
	if end.Before(s.segStart) {
		end = s.segStart
	}
	apps := s.takeAppSummaries(end)
	sum := domain.SessionSummary{
		SessionID:  s.sessionID,
		Username:   s.username,
		Login:      s.segStart,
		DurationMs: end.Sub(s.segStart).Milliseconds(),
		Apps:       apps,
	}
	if !stillOpen {
		logout := end
		sum.Logout = &logout
	}
	s.segStart = time.Time{}
	return &sum
}

func (s *sessAcc) lockedSummary(now time.Time, stillOpen bool) domain.SessionSummary {
	start := now
	if s.lockedAt != nil {
		start = *s.lockedAt
	}
	end := now
	sum := domain.SessionSummary{
		SessionID:  s.sessionID,
		Username:   s.username,
		Login:      start,
		DurationMs: end.Sub(start).Milliseconds(),
		LockedMs:   end.Sub(start).Milliseconds(),
		LockedNow:  stillOpen,
	}
	if !stillOpen {
		logout := end
		sum.Logout = &logout
		sum.LockedNow = false
	}
	return sum
}

func newSessAcc(sid uint32, username string, login time.Time) *sessAcc {
	return &sessAcc{
		sessionID: sid,
		username:  username,
		segStart:  login,
		appsOpen:  map[string]*openApp{},
		appTotals: map[string]*domain.AppSummary{},
	}
}

// AggregateDayActivity builds usage segments (split on lock/unlock) with nested per-app totals.
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

	openByID := map[uint32]*sessAcc{}
	var sessions []domain.SessionSummary

	appendSeg := func(sum *domain.SessionSummary) {
		if sum != nil {
			sessions = append(sessions, *sum)
		}
	}

	findByTime := func(username string, ts time.Time) *sessAcc {
		for _, s := range openByID {
			if username != "" && s.username != "" && !strings.EqualFold(s.username, username) {
				continue
			}
			if s.unlocked() && !ts.Before(s.segStart) {
				return s
			}
		}
		return nil
	}

	resolve := func(ev domain.ActivityEvent) *sessAcc {
		if ev.SessionID != 0 {
			if s := openByID[ev.SessionID]; s != nil && s.unlocked() {
				if ev.Username == "" || strings.EqualFold(s.username, ev.Username) {
					return s
				}
			}
		}
		return findByTime(ev.Username, ev.Timestamp)
	}

	closeOpen := func(s *sessAcc, at time.Time) {
		if s.lockedAt != nil {
			sessions = append(sessions, s.lockedSummary(at, false))
			s.lockedAt = nil
		} else {
			appendSeg(s.flushUnlocked(at, false))
		}
	}

	for _, ev := range sorted {
		ts := ev.Timestamp
		switch ev.Type {
		case domain.EventSessionLogin:
			if prev := openByID[ev.SessionID]; prev != nil {
				if strings.EqualFold(prev.username, ev.Username) {
					// Duplicate login (e.g. client restart) — keep the open session.
					continue
				}
				closeOpen(prev, ts)
				delete(openByID, ev.SessionID)
			}
			// Close earlier open sessions for the same user (ghost / switch).
			for id, s := range openByID {
				if ev.Username != "" && strings.EqualFold(s.username, ev.Username) {
					closeOpen(s, ts)
					delete(openByID, id)
				}
			}
			openByID[ev.SessionID] = newSessAcc(ev.SessionID, ev.Username, ts)

		case domain.EventSessionLock:
			s := openByID[ev.SessionID]
			if s == nil || s.lockedAt != nil {
				continue
			}
			appendSeg(s.flushUnlocked(ts, false))
			t := ts
			s.lockedAt = &t

		case domain.EventSessionUnlock:
			s := openByID[ev.SessionID]
			if s == nil || s.lockedAt == nil {
				continue
			}
			// Lock gap is visible between segments; do not emit a lock-only card after unlock.
			s.lockedAt = nil
			s.segStart = ts
			s.resetApps()

		case domain.EventSessionLogout:
			s := openByID[ev.SessionID]
			if s == nil {
				logout := ts
				sessions = append(sessions, domain.SessionSummary{
					SessionID:  ev.SessionID,
					Username:   ev.Username,
					Login:      ts,
					Logout:     &logout,
					DurationMs: 0,
				})
				continue
			}
			closeOpen(s, ts)
			delete(openByID, ev.SessionID)

		case domain.EventAppOpen:
			s := resolve(ev)
			if s == nil {
				continue
			}
			k := appKey(ev.AppName, ev.ExePath)
			s.appsOpen[k] = &openApp{name: ev.AppName, exe: ev.ExePath, opened: ts}
			s.ensureApp(ev.AppName, ev.ExePath)
		case domain.EventAppClose:
			s := resolve(ev)
			if s == nil {
				continue
			}
			k := appKey(ev.AppName, ev.ExePath)
			a := s.ensureApp(ev.AppName, ev.ExePath)
			if o, ok := s.appsOpen[k]; ok {
				dur := ev.DurationMs
				if dur <= 0 {
					dur = ts.Sub(o.opened).Milliseconds()
				}
				a.OpenMs += dur
				delete(s.appsOpen, k)
			} else if ev.DurationMs > 0 {
				a.OpenMs += ev.DurationMs
			}
			if s.focusStart != nil && appKey(s.focusApp, s.focusExe) == k {
				s.closeFocus(ts)
			}
		case domain.EventAppFocus:
			s := resolve(ev)
			if s == nil {
				continue
			}
			s.closeFocus(ts)
			t := ts
			s.focusStart = &t
			s.focusApp = ev.AppName
			s.focusExe = ev.ExePath
			s.ensureApp(ev.AppName, ev.ExePath)
		case domain.EventAppBlur:
			s := resolve(ev)
			if s == nil || s.focusStart == nil {
				continue
			}
			if ev.DurationMs > 0 {
				s.ensureApp(s.focusApp, s.focusExe).FocusMs += ev.DurationMs
				s.focusStart = nil
			} else {
				s.closeFocus(ts)
			}
		}
	}

	for _, s := range openByID {
		if s.lockedAt != nil {
			sessions = append(sessions, s.lockedSummary(now, true))
			continue
		}
		appendSeg(s.flushUnlocked(now, true))
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Login.Before(sessions[j].Login)
	})

	return domain.DayActivity{
		Date:     dayStr,
		Sessions: sessions,
	}
}
