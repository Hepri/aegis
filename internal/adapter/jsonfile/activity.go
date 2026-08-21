package jsonfile

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

const activityRetentionDays = 90

// ActivityStore writes events as JSONL under activity/{clientID}/{YYYY-MM-DD}.jsonl
type ActivityStore struct {
	mu      sync.Mutex
	baseDir string
	loc     *time.Location
}

func NewActivityStore(dataFilePath string, loc *time.Location) *ActivityStore {
	if loc == nil {
		loc = time.UTC
	}
	base := filepath.Join(filepath.Dir(dataFilePath), "activity")
	return &ActivityStore{baseDir: base, loc: loc}
}

func (s *ActivityStore) dayFile(clientID string, t time.Time) string {
	day := t.In(s.loc).Format("2006-01-02")
	return filepath.Join(s.baseDir, clientID, day+".jsonl")
}

func (s *ActivityStore) AppendEvents(ctx context.Context, clientID string, events []domain.ActivityEvent) error {
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	byDay := make(map[string][]domain.ActivityEvent)
	for _, ev := range events {
		ts := ev.Timestamp
		if ts.IsZero() {
			ts = time.Now().In(s.loc)
			ev.Timestamp = ts
		}
		day := ts.In(s.loc).Format("2006-01-02")
		byDay[day] = append(byDay[day], ev)
	}

	for day, dayEvents := range byDay {
		path := filepath.Join(s.baseDir, clientID, day+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		for _, ev := range dayEvents {
			if err := enc.Encode(ev); err != nil {
				f.Close()
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	// Best-effort retention cleanup (don't fail ingest)
	_ = s.cleanupLocked(activityRetentionDays)
	return nil
}

func (s *ActivityStore) ReadDayEvents(ctx context.Context, clientID string, day time.Time) ([]domain.ActivityEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.dayFile(clientID, day)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []domain.ActivityEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev domain.ActivityEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, sc.Err()
}

func (s *ActivityStore) CleanupOlderThan(ctx context.Context, retentionDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupLocked(retentionDays)
}

func (s *ActivityStore) cleanupLocked(retentionDays int) error {
	cutoff := time.Now().In(s.loc).AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, clientDir := range entries {
		if !clientDir.IsDir() {
			continue
		}
		dirPath := filepath.Join(s.baseDir, clientDir.Name())
		files, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			dayStr := strings.TrimSuffix(f.Name(), ".jsonl")
			day, err := time.ParseInLocation("2006-01-02", dayStr, s.loc)
			if err != nil {
				continue
			}
			if day.Before(cutoff) {
				_ = os.Remove(filepath.Join(dirPath, f.Name()))
			}
		}
	}
	return nil
}

// PresenceStore keeps last_seen + client binary version in a small JSON file.
type PresenceStore struct {
	mu       sync.RWMutex
	presence map[string]port.ClientPresence
	filePath string
}

func NewPresenceStore(dataFilePath string) *PresenceStore {
	p := &PresenceStore{
		presence: make(map[string]port.ClientPresence),
		filePath: filepath.Join(filepath.Dir(dataFilePath), "last-seen.json"),
	}
	_ = p.load()
	return p
}

func (p *PresenceStore) load() error {
	data, err := os.ReadFile(p.filePath)
	if err != nil {
		return err
	}
	// New format: map[clientID]ClientPresence
	var m map[string]port.ClientPresence
	if err := json.Unmarshal(data, &m); err == nil && m != nil {
		// Detect legacy map[string]time.Time (values would unmarshal with zero LastSeen if wrong)
		legacy := map[string]time.Time{}
		if err2 := json.Unmarshal(data, &legacy); err2 == nil {
			// If any entry has empty LastSeen but legacy parse has times, migrate
			needsMigrate := false
			for id, pr := range m {
				if pr.LastSeen.IsZero() {
					if t, ok := legacy[id]; ok && !t.IsZero() {
						needsMigrate = true
						break
					}
				}
			}
			if needsMigrate || len(m) == 0 && len(legacy) > 0 {
				m = make(map[string]port.ClientPresence, len(legacy))
				for id, t := range legacy {
					m[id] = port.ClientPresence{LastSeen: t}
				}
			}
		}
		p.mu.Lock()
		p.presence = m
		p.mu.Unlock()
		return nil
	}
	// Pure legacy: {"id": "2026-..."}
	var legacy map[string]time.Time
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.presence = make(map[string]port.ClientPresence, len(legacy))
	for id, t := range legacy {
		p.presence[id] = port.ClientPresence{LastSeen: t}
	}
	return nil
}

func (p *PresenceStore) saveLocked() {
	data, err := json.MarshalIndent(p.presence, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p.filePath), 0755)
	_ = os.WriteFile(p.filePath, data, 0644)
}

func (p *PresenceStore) TouchPresence(ctx context.Context, clientID, clientVersion string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr := p.presence[clientID]
	pr.LastSeen = time.Now().UTC()
	if clientVersion != "" {
		pr.ClientVersion = clientVersion
	}
	p.presence[clientID] = pr
	p.saveLocked()
	return nil
}

func (p *PresenceStore) GetPresence(ctx context.Context, clientID string) (port.ClientPresence, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pr, ok := p.presence[clientID]
	return pr, ok
}

func (p *PresenceStore) GetAllPresence(ctx context.Context) map[string]port.ClientPresence {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]port.ClientPresence, len(p.presence))
	for k, v := range p.presence {
		out[k] = v
	}
	return out
}
