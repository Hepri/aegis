package server

import (
	"sort"
	"strings"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

type sessAcc struct {
	sessionID  uint32
	username   string
	login      time.Time
	logout     *time.Time
	lockedAt   *time.Time
	lockedMs   int64
	appsOpen   map[string]*openApp
	appTotals  map[string]*domain.AppSummary
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

func (s *sessAcc) toSummary(now time.Time) domain.SessionSummary {
	end := now
	if s.logout != nil {
		end = *s.logout
	}
	if s.lockedAt != nil {
		s.lockedMs += end.Sub(*s.lockedAt).Milliseconds()
		s.lockedAt = nil
	}
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

	return domain.SessionSummary{
		SessionID:  s.sessionID,
		Username:   s.username,
		Login:      s.login,
		Logout:     s.logout,
		DurationMs: end.Sub(s.login).Milliseconds(),
		LockedMs:   s.lockedMs,
		Apps:       apps,
	}
}

func newSessAcc(sid uint32, username string, login time.Time) *sessAcc {
	return &sessAcc{
		sessionID: sid,
		username:  username,
		login:     login,
		appsOpen:  map[string]*openApp{},
		appTotals: map[string]*domain.AppSummary{},
	}
}

// AggregateDayActivity builds sessions with nested per-app totals.
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
	var closed []*sessAcc
	var orphan *sessAcc // app events before any matching session

	findByTime := func(username string, ts time.Time) *sessAcc {
		for _, s := range openByID {
			if username != "" && s.username != "" && !strings.EqualFold(s.username, username) {
				continue
			}
			if !ts.Before(s.login) {
				return s
			}
		}
		for i := len(closed) - 1; i >= 0; i-- {
			s := closed[i]
			if username != "" && s.username != "" && !strings.EqualFold(s.username, username) {
				continue
			}
			if ts.Before(s.login) {
				continue
			}
			if s.logout != nil && ts.After(*s.logout) {
				continue
			}
			return s
		}
		return nil
	}

	resolve := func(ev domain.ActivityEvent) *sessAcc {
		if ev.SessionID != 0 {
			if s := openByID[ev.SessionID]; s != nil {
				return s
			}
			for _, s := range closed {
				if s.sessionID == ev.SessionID {
					return s
				}
			}
		}
		if s := findByTime(ev.Username, ev.Timestamp); s != nil {
			return s
		}
		if orphan == nil {
			orphan = newSessAcc(0, ev.Username, ev.Timestamp)
		}
		return orphan
	}

	for _, ev := range sorted {
		ts := ev.Timestamp
		switch ev.Type {
		case domain.EventSessionLogin:
			openByID[ev.SessionID] = newSessAcc(ev.SessionID, ev.Username, ts)

		case domain.EventSessionLock:
			if s := openByID[ev.SessionID]; s != nil && s.lockedAt == nil {
				t := ts
				s.lockedAt = &t
			}
		case domain.EventSessionUnlock:
			if s := openByID[ev.SessionID]; s != nil && s.lockedAt != nil {
				s.lockedMs += ts.Sub(*s.lockedAt).Milliseconds()
				s.lockedAt = nil
			}
		case domain.EventSessionLogout:
			s := openByID[ev.SessionID]
			if s == nil {
				s = newSessAcc(ev.SessionID, ev.Username, ts)
				logout := ts
				s.logout = &logout
				closed = append(closed, s)
				continue
			}
			logout := ts
			s.logout = &logout
			closed = append(closed, s)
			delete(openByID, ev.SessionID)

		case domain.EventAppOpen:
			s := resolve(ev)
			k := appKey(ev.AppName, ev.ExePath)
			s.appsOpen[k] = &openApp{name: ev.AppName, exe: ev.ExePath, opened: ts}
			s.ensureApp(ev.AppName, ev.ExePath)
		case domain.EventAppClose:
			s := resolve(ev)
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
			s.closeFocus(ts)
			t := ts
			s.focusStart = &t
			s.focusApp = ev.AppName
			s.focusExe = ev.ExePath
			s.ensureApp(ev.AppName, ev.ExePath)
		case domain.EventAppBlur:
			s := resolve(ev)
			if s.focusStart == nil {
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

	var sessions []domain.SessionSummary
	for _, s := range closed {
		sessions = append(sessions, s.toSummary(now))
	}
	for _, s := range openByID {
		sessions = append(sessions, s.toSummary(now))
	}
	if orphan != nil && len(orphan.appTotals) > 0 {
		if orphan.username == "" {
			orphan.username = "—"
		}
		sessions = append(sessions, orphan.toSummary(now))
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Login.Before(sessions[j].Login)
	})

	return domain.DayActivity{
		Date:     dayStr,
		Sessions: sessions,
	}
}
