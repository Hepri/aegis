package jsonfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
	"github.com/aegis/parental-control/internal/usecase/server"
	"github.com/google/uuid"
)

const maxRequests = 10

type persistedBlockRequest struct {
	ID     string    `json:"id"`
	UserID string    `json:"user_id,omitempty"`
	Start  time.Time `json:"start"`
	Until  time.Time `json:"until"`
}

type persistedTempAccessRequest struct {
	ID     string    `json:"id"`
	UserID string    `json:"user_id"`
	Start  time.Time `json:"start"`
	Until  time.Time `json:"until"`
}

type persistedClient struct {
	ID                      string                       `json:"id"`
	Name                    string                       `json:"name"`
	Users                   []persistedUser              `json:"users"`
	BlockRequests           []persistedBlockRequest      `json:"block_requests,omitempty"`
	TemporaryAccessRequests []persistedTempAccessRequest `json:"temporary_access_requests,omitempty"`
}

type persistedUser struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Username string             `json:"username"`
	Schedule domain.DaySchedule `json:"schedule"`
}

type persistedData struct {
	Clients map[string]persistedClient `json:"clients"`
}

type Repository struct {
	mu          sync.RWMutex
	filePath    string
	clients     map[string]*clientState
	subscribers map[string][]chan struct{}
	subMu       sync.Mutex
}

type clientState struct {
	ID                      string
	Name                    string
	Users                   []domain.User
	BlockRequests           []port.BlockRequest
	TemporaryAccessRequests []port.TemporaryAccessRequest
	LastSentIntervals       map[string][]domain.AllowedInterval
	LastSentVersion         string
	ComputedConfig          *domain.ClientConfig
}

func New(filePath string) (*Repository, error) {
	r := &Repository{
		filePath:    filePath,
		clients:     make(map[string]*clientState),
		subscribers: make(map[string][]chan struct{}),
	}
	if err := r.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return r, nil
}

func (r *Repository) load() error {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return err
	}
	var pd persistedData
	if err := json.Unmarshal(data, &pd); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, pc := range pd.Clients {
		users := make([]domain.User, 0, len(pc.Users))
		for _, pu := range pc.Users {
			users = append(users, domain.User{
				ID:       pu.ID,
				Name:     pu.Name,
				Username: pu.Username,
				Schedule: pu.Schedule,
			})
		}
		blockReqs := make([]port.BlockRequest, 0, len(pc.BlockRequests))
		for _, b := range pc.BlockRequests {
			id := b.ID
			if id == "" {
				id = uuid.New().String()
			}
			blockReqs = append(blockReqs, port.BlockRequest{ID: id, UserID: b.UserID, Start: b.Start, Until: b.Until})
		}
		tempReqs := make([]port.TemporaryAccessRequest, 0, len(pc.TemporaryAccessRequests))
		for _, t := range pc.TemporaryAccessRequests {
			id := t.ID
			if id == "" {
				id = uuid.New().String()
			}
			tempReqs = append(tempReqs, port.TemporaryAccessRequest{ID: id, UserID: t.UserID, Start: t.Start, Until: t.Until})
		}
		r.clients[id] = &clientState{
			ID:                      pc.ID,
			Name:                    pc.Name,
			Users:                   users,
			BlockRequests:           blockReqs,
			TemporaryAccessRequests: tempReqs,
		}
	}
	return nil
}

func (r *Repository) save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.saveLocked()
}

func (r *Repository) saveLocked() error {
	pd := persistedData{
		Clients: make(map[string]persistedClient),
	}
	for id, cs := range r.clients {
		users := make([]persistedUser, 0, len(cs.Users))
		for _, u := range cs.Users {
			users = append(users, persistedUser{
				ID:       u.ID,
				Name:     u.Name,
				Username: u.Username,
				Schedule: u.Schedule,
			})
		}
		blockReqs := make([]persistedBlockRequest, 0, len(cs.BlockRequests))
		for _, b := range cs.BlockRequests {
			id := b.ID
			if id == "" {
				id = uuid.New().String()
			}
			blockReqs = append(blockReqs, persistedBlockRequest{ID: id, UserID: b.UserID, Start: b.Start, Until: b.Until})
		}
		tempReqs := make([]persistedTempAccessRequest, 0, len(cs.TemporaryAccessRequests))
		for _, t := range cs.TemporaryAccessRequests {
			id := t.ID
			if id == "" {
				id = uuid.New().String()
			}
			tempReqs = append(tempReqs, persistedTempAccessRequest{ID: id, UserID: t.UserID, Start: t.Start, Until: t.Until})
		}
		pd.Clients[id] = persistedClient{
			ID:                      id,
			Name:                    cs.Name,
			Users:                   users,
			BlockRequests:           blockReqs,
			TemporaryAccessRequests: tempReqs,
		}
	}

	data, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(r.filePath, data, 0644)
}

func (r *Repository) notify(clientID string) {
	r.subMu.Lock()
	chans := r.subscribers[clientID]
	r.subMu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (r *Repository) GetClient(ctx context.Context, clientID string) (*port.ClientState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil, nil
	}
	// Clean up expired temp access and blocks
	now := time.Now()
	needsSave := false

	// Filter expired temporary access
	validTemp := cs.TemporaryAccessRequests[:0]
	for _, t := range cs.TemporaryAccessRequests {
		if t.Until.After(now) {
			validTemp = append(validTemp, t)
		} else {
			needsSave = true
		}
	}
	cs.TemporaryAccessRequests = validTemp

	// Filter expired blocks
	validBlocks := cs.BlockRequests[:0]
	for _, b := range cs.BlockRequests {
		if b.Until.After(now) {
			validBlocks = append(validBlocks, b)
		} else {
			needsSave = true
		}
	}
	cs.BlockRequests = validBlocks

	// Compute config if missing (migration for existing clients)
	if cs.ComputedConfig == nil {
		if cs.LastSentVersion == "" {
			cs.LastSentVersion = uuid.New().String()
		}
		state := r.toPortState(cs)
		config, _ := server.ComputeClientConfig(time.Now(), state, true)
		cs.ComputedConfig = &config
		needsSave = true
	}

	if needsSave {
		r.saveLocked()
	}

	return r.toPortState(cs), nil
}

func (r *Repository) toPortState(cs *clientState) *port.ClientState {
	users := make([]domain.User, len(cs.Users))
	copy(users, cs.Users)
	blockReqs := make([]port.BlockRequest, len(cs.BlockRequests))
	copy(blockReqs, cs.BlockRequests)
	tempReqs := make([]port.TemporaryAccessRequest, len(cs.TemporaryAccessRequests))
	copy(tempReqs, cs.TemporaryAccessRequests)
	lastSent := make(map[string][]domain.AllowedInterval)
	for k, v := range cs.LastSentIntervals {
		lastSent[k] = append([]domain.AllowedInterval(nil), v...)
	}
	return &port.ClientState{
		ID:                      cs.ID,
		Name:                    cs.Name,
		Users:                   users,
		BlockRequests:           blockReqs,
		TemporaryAccessRequests: tempReqs,
		LastSentIntervals:       lastSent,
		LastSentVersion:         cs.LastSentVersion,
		ComputedConfig:          cs.ComputedConfig,
	}
}

func (r *Repository) GetAllClients(ctx context.Context) ([]*port.ClientState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*port.ClientState
	for _, cs := range r.clients {
		result = append(result, r.toPortState(cs))
	}
	return result, nil
}

func (r *Repository) SaveClient(ctx context.Context, client *port.ClientState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate version if missing
	if client.LastSentVersion == "" {
		client.LastSentVersion = uuid.New().String()
	}

	// Compute config on save
	config, _ := server.ComputeClientConfig(time.Now(), client, true)

	cs := &clientState{
		ID:                      client.ID,
		Name:                    client.Name,
		Users:                   append([]domain.User(nil), client.Users...),
		BlockRequests:           append([]port.BlockRequest(nil), client.BlockRequests...),
		TemporaryAccessRequests: append([]port.TemporaryAccessRequest(nil), client.TemporaryAccessRequests...),
		LastSentIntervals:       client.LastSentIntervals,
		LastSentVersion:         client.LastSentVersion,
		ComputedConfig:          &config,
	}
	r.clients[client.ID] = cs
	return r.saveLocked()
}

func (r *Repository) DeleteClient(ctx context.Context, clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[clientID]; !ok {
		return nil
	}
	delete(r.clients, clientID)
	r.subMu.Lock()
	delete(r.subscribers, clientID)
	r.subMu.Unlock()
	return r.saveLocked()
}

func (r *Repository) AddUser(ctx context.Context, clientID string, user domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		cs = &clientState{
			ID:                      clientID,
			Name:                    clientID,
			Users:                   nil,
			BlockRequests:           nil,
			TemporaryAccessRequests: nil,
		}
		r.clients[clientID] = cs
	}
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	cs.Users = append(cs.Users, user)
	// Recompute config
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(time.Now(), state, true)
	cs.ComputedConfig = &config
	r.notify(clientID)
	return r.saveLocked()
}

func (r *Repository) UpdateUserSchedule(ctx context.Context, clientID, userID string, schedule domain.DaySchedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			cs.Users[i].Schedule = schedule
			// Recompute config
			state := r.toPortState(cs)
			config, _ := server.ComputeClientConfig(time.Now(), state, true)
			cs.ComputedConfig = &config
			r.notify(clientID)
			return r.saveLocked()
		}
	}
	return nil
}

func (r *Repository) DeleteUser(ctx context.Context, clientID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	for i, u := range cs.Users {
		if u.ID == userID {
			cs.Users = append(cs.Users[:i], cs.Users[i+1:]...)
			// Remove temp access for deleted user
			newTemp := cs.TemporaryAccessRequests[:0]
			for _, t := range cs.TemporaryAccessRequests {
				if t.UserID != userID {
					newTemp = append(newTemp, t)
				}
			}
			cs.TemporaryAccessRequests = newTemp
			// Recompute config
			state := r.toPortState(cs)
			config, _ := server.ComputeClientConfig(time.Now(), state, true)
			cs.ComputedConfig = &config
			r.notify(clientID)
			return r.saveLocked()
		}
	}
	return nil
}

func (r *Repository) GrantTemporaryAccess(ctx context.Context, clientID, userID string, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	now := time.Now()
	cs.TemporaryAccessRequests = append(cs.TemporaryAccessRequests, port.TemporaryAccessRequest{ID: uuid.New().String(), UserID: userID, Start: now, Until: until})
	if len(cs.TemporaryAccessRequests) > maxRequests {
		cs.TemporaryAccessRequests = cs.TemporaryAccessRequests[len(cs.TemporaryAccessRequests)-maxRequests:]
	}
	// Recompute config
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(time.Now(), state, true)
	cs.ComputedConfig = &config
	r.notify(clientID)
	return r.saveLocked()
}

func (r *Repository) BlockClient(ctx context.Context, clientID, userID string, start, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	cs.BlockRequests = append(cs.BlockRequests, port.BlockRequest{ID: uuid.New().String(), UserID: userID, Start: start, Until: until})
	if len(cs.BlockRequests) > maxRequests {
		cs.BlockRequests = cs.BlockRequests[len(cs.BlockRequests)-maxRequests:]
	}
	// Recompute config
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(time.Now(), state, true)
	cs.ComputedConfig = &config
	r.notify(clientID)
	return r.saveLocked()
}

func (r *Repository) DeleteBlockRequest(ctx context.Context, clientID, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	for i, b := range cs.BlockRequests {
		if b.ID == requestID {
			cs.BlockRequests = append(cs.BlockRequests[:i], cs.BlockRequests[i+1:]...)
			// Recompute config
			state := r.toPortState(cs)
			config, _ := server.ComputeClientConfig(time.Now(), state, true)
			cs.ComputedConfig = &config
			r.notify(clientID)
			return r.saveLocked()
		}
	}
	return nil
}

func (r *Repository) DeleteTemporaryAccessRequest(ctx context.Context, clientID, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	for i, t := range cs.TemporaryAccessRequests {
		if t.ID == requestID {
			cs.TemporaryAccessRequests = append(cs.TemporaryAccessRequests[:i], cs.TemporaryAccessRequests[i+1:]...)
			// Recompute config
			state := r.toPortState(cs)
			config, _ := server.ComputeClientConfig(time.Now(), state, true)
			cs.ComputedConfig = &config
			r.notify(clientID)
			return r.saveLocked()
		}
	}
	return nil
}

func (r *Repository) UpdateLastSent(ctx context.Context, clientID string, intervals map[string][]domain.AllowedInterval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	cs.LastSentIntervals = make(map[string][]domain.AllowedInterval)
	for k, v := range intervals {
		cs.LastSentIntervals[k] = append([]domain.AllowedInterval(nil), v...)
	}
	return nil
}

func (r *Repository) IncrementConfigVersion(ctx context.Context, clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil
	}
	cs.LastSentVersion = uuid.New().String()
	// Recompute config for today+tomorrow
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(time.Now(), state, true)
	cs.ComputedConfig = &config
	r.notify(clientID)
	return r.saveLocked()
}

func (r *Repository) Subscribe(ctx context.Context, clientID string) <-chan struct{} {
	ch := make(chan struct{}, 1)
	r.subMu.Lock()
	r.subscribers[clientID] = append(r.subscribers[clientID], ch)
	r.subMu.Unlock()
	return ch
}
