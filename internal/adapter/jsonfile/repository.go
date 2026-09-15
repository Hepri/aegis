package jsonfile

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
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
	Source string    `json:"source,omitempty"`
}

type persistedClient struct {
	ID                      string                       `json:"id"`
	Name                    string                       `json:"name"`
	Users                   []persistedUser              `json:"users"`
	BlockRequests           []persistedBlockRequest      `json:"block_requests,omitempty"`
	TemporaryAccessRequests []persistedTempAccessRequest `json:"temporary_access_requests,omitempty"`
	EarnTasks               []domain.EarnTask            `json:"earn_tasks,omitempty"`
	EarnSettings            *domain.EarnSettings         `json:"earn_settings,omitempty"`
	EarnLog                 []domain.EarnLogEntry        `json:"earn_log,omitempty"`
}

type persistedUser struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Username            string                 `json:"username"`
	Schedule            domain.DaySchedule     `json:"schedule"`
	EarnBalanceMinutes  int                    `json:"earn_balance_minutes,omitempty"`
	SolvedTaskIDs       []string               `json:"solved_task_ids,omitempty"`
	EarnDayStats        *domain.EarnDayStats   `json:"earn_day_stats,omitempty"`
	EarnLockedUntil     int64                  `json:"earn_locked_until,omitempty"`
	ActiveEarnChallenge *domain.EarnChallenge  `json:"active_earn_challenge,omitempty"`
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
	loc         *time.Location
}

type clientState struct {
	ID                      string
	Name                    string
	Users                   []domain.User
	BlockRequests           []port.BlockRequest
	TemporaryAccessRequests []port.TemporaryAccessRequest
	EarnTasks               []domain.EarnTask
	EarnSettings            domain.EarnSettings
	EarnLog                 []domain.EarnLogEntry
	LastSentIntervals       map[string][]domain.AllowedInterval
	LastSentVersion         string
	ComputedConfig          *domain.ClientConfig
}

func New(filePath string, loc *time.Location) (*Repository, error) {
	if loc == nil {
		loc = time.UTC
	}
	r := &Repository{
		filePath:    filePath,
		clients:     make(map[string]*clientState),
		subscribers: make(map[string][]chan struct{}),
		loc:         loc,
	}
	if err := r.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return r, nil
}

func (r *Repository) now() time.Time {
	return time.Now().In(r.loc)
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
			u := domain.User{
				ID:                  pu.ID,
				Name:                pu.Name,
				Username:            pu.Username,
				Schedule:            pu.Schedule,
				EarnBalanceMinutes:  pu.EarnBalanceMinutes,
				SolvedTaskIDs:       append([]string(nil), pu.SolvedTaskIDs...),
				EarnLockedUntil:     pu.EarnLockedUntil,
				ActiveEarnChallenge: cloneChallenge(pu.ActiveEarnChallenge),
			}
			if pu.EarnDayStats != nil {
				u.EarnDayStats = *pu.EarnDayStats
			}
			users = append(users, u)
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
			tempReqs = append(tempReqs, port.TemporaryAccessRequest{ID: id, UserID: t.UserID, Start: t.Start, Until: t.Until, Source: t.Source})
		}
		settings := domain.DefaultEarnSettings()
		if pc.EarnSettings != nil {
			settings = *pc.EarnSettings
		}
		tasks := append([]domain.EarnTask(nil), pc.EarnTasks...)
		if tasks == nil {
			tasks = []domain.EarnTask{}
		}
		r.clients[id] = &clientState{
			ID:                      pc.ID,
			Name:                    pc.Name,
			Users:                   users,
			BlockRequests:           blockReqs,
			TemporaryAccessRequests: tempReqs,
			EarnTasks:               tasks,
			EarnSettings:            settings,
			EarnLog:                 append([]domain.EarnLogEntry(nil), pc.EarnLog...),
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
			pu := persistedUser{
				ID:                  u.ID,
				Name:                u.Name,
				Username:            u.Username,
				Schedule:            u.Schedule,
				EarnBalanceMinutes:  u.EarnBalanceMinutes,
				SolvedTaskIDs:       append([]string(nil), u.SolvedTaskIDs...),
				EarnLockedUntil:     u.EarnLockedUntil,
				ActiveEarnChallenge: cloneChallenge(u.ActiveEarnChallenge),
			}
			if u.EarnDayStats.Date != "" || u.EarnDayStats.EarnedMinutes > 0 || u.EarnDayStats.SpentMinutes > 0 || u.EarnDayStats.SolvedCount > 0 {
				stats := u.EarnDayStats
				pu.EarnDayStats = &stats
			}
			users = append(users, pu)
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
			tempReqs = append(tempReqs, persistedTempAccessRequest{ID: id, UserID: t.UserID, Start: t.Start, Until: t.Until, Source: t.Source})
		}
		settings := cs.EarnSettings
		pd.Clients[id] = persistedClient{
			ID:                      id,
			Name:                    cs.Name,
			Users:                   users,
			BlockRequests:           blockReqs,
			TemporaryAccessRequests: tempReqs,
			EarnTasks:               append([]domain.EarnTask(nil), cs.EarnTasks...),
			EarnSettings:            &settings,
			EarnLog:                 append([]domain.EarnLogEntry(nil), cs.EarnLog...),
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
	now := r.now()
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
		config, _ := server.ComputeClientConfig(r.now(), state, true)
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
	for i := range users {
		users[i].SolvedTaskIDs = append([]string(nil), cs.Users[i].SolvedTaskIDs...)
		users[i].ActiveEarnChallenge = cloneChallenge(cs.Users[i].ActiveEarnChallenge)
	}
	blockReqs := make([]port.BlockRequest, len(cs.BlockRequests))
	copy(blockReqs, cs.BlockRequests)
	tempReqs := make([]port.TemporaryAccessRequest, len(cs.TemporaryAccessRequests))
	copy(tempReqs, cs.TemporaryAccessRequests)
	lastSent := make(map[string][]domain.AllowedInterval)
	for k, v := range cs.LastSentIntervals {
		lastSent[k] = append([]domain.AllowedInterval(nil), v...)
	}
	tasks := append([]domain.EarnTask(nil), cs.EarnTasks...)
	if tasks == nil {
		tasks = []domain.EarnTask{}
	}
	return &port.ClientState{
		ID:                      cs.ID,
		Name:                    cs.Name,
		Users:                   users,
		BlockRequests:           blockReqs,
		TemporaryAccessRequests: tempReqs,
		EarnTasks:               tasks,
		EarnSettings:            cs.EarnSettings,
		EarnLog:                 append([]domain.EarnLogEntry(nil), cs.EarnLog...),
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
	config, _ := server.ComputeClientConfig(r.now(), client, true)

	cs := &clientState{
		ID:                      client.ID,
		Name:                    client.Name,
		Users:                   append([]domain.User(nil), client.Users...),
		BlockRequests:           append([]port.BlockRequest(nil), client.BlockRequests...),
		TemporaryAccessRequests: append([]port.TemporaryAccessRequest(nil), client.TemporaryAccessRequests...),
		EarnTasks:               append([]domain.EarnTask(nil), client.EarnTasks...),
		EarnSettings:            client.EarnSettings,
		EarnLog:                 append([]domain.EarnLogEntry(nil), client.EarnLog...),
		LastSentIntervals:       client.LastSentIntervals,
		LastSentVersion:         client.LastSentVersion,
		ComputedConfig:          &config,
	}
	if cs.EarnSettings.DefaultRewardMinutes == 0 && cs.EarnSettings.MaxEarnPerDay == 0 {
		cs.EarnSettings = domain.DefaultEarnSettings()
	}
	if cs.EarnTasks == nil {
		cs.EarnTasks = []domain.EarnTask{}
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
	config, _ := server.ComputeClientConfig(r.now(), state, true)
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
			config, _ := server.ComputeClientConfig(r.now(), state, true)
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
			config, _ := server.ComputeClientConfig(r.now(), state, true)
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
	now := r.now()
	cs.TemporaryAccessRequests = append(cs.TemporaryAccessRequests, port.TemporaryAccessRequest{ID: uuid.New().String(), UserID: userID, Start: now, Until: until})
	if len(cs.TemporaryAccessRequests) > maxRequests {
		cs.TemporaryAccessRequests = cs.TemporaryAccessRequests[len(cs.TemporaryAccessRequests)-maxRequests:]
	}
	// Recompute config
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(r.now(), state, true)
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
	config, _ := server.ComputeClientConfig(r.now(), state, true)
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
			config, _ := server.ComputeClientConfig(r.now(), state, true)
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
			config, _ := server.ComputeClientConfig(r.now(), state, true)
			cs.ComputedConfig = &config
			r.notify(clientID)
			return r.saveLocked()
		}
	}
	return nil
}

func (r *Repository) UpdateEarnSettings(ctx context.Context, clientID string, settings domain.EarnSettings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.ErrClientNotFound
	}
	normalized := domain.NormalizeEarnSettings(settings)
	normalized.MathGeneratorEnabled = settings.MathGeneratorEnabled
	normalized.DisableRedeem = settings.DisableRedeem
	normalized.RedeemSchedule = domain.ResolveRedeemSchedule(settings.RedeemSchedule)
	normalized.SubjectRewardMinutes = domain.ResolveSubjectRewardMinutes(settings.SubjectRewardMinutes)
	normalized.SubjectWrongPenaltyMinutes = domain.ResolveSubjectWrongPenaltyMinutes(settings.SubjectWrongPenaltyMinutes)
	cs.EarnSettings = normalized
	return r.saveLocked()
}

func (r *Repository) UpsertEarnTask(ctx context.Context, clientID string, task domain.EarnTask) (domain.EarnTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.EarnTask{}, domain.ErrClientNotFound
	}
	task, err := domain.NormalizeCustomEarnTask(task)
	if err != nil {
		return domain.EarnTask{}, err
	}
	if task.ID == "" {
		task.ID = "custom-" + uuid.New().String()
	} else if domain.IsBuiltinEarnTaskID(task.ID) {
		return domain.EarnTask{}, domain.ErrBuiltinTask
	}
	found := false
	for i := range cs.EarnTasks {
		if cs.EarnTasks[i].ID == task.ID {
			cs.EarnTasks[i] = task
			found = true
			break
		}
	}
	if !found {
		cs.EarnTasks = append(cs.EarnTasks, task)
	}
	if err := r.saveLocked(); err != nil {
		return domain.EarnTask{}, err
	}
	return task, nil
}

func (r *Repository) DeleteEarnTask(ctx context.Context, clientID, taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.ErrClientNotFound
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return domain.ErrTaskNotFound
	}
	if domain.IsBuiltinEarnTaskID(taskID) {
		return domain.ErrBuiltinTask
	}
	out := make([]domain.EarnTask, 0, len(cs.EarnTasks))
	found := false
	for _, t := range cs.EarnTasks {
		if t.ID == taskID {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return domain.ErrTaskNotFound
	}
	cs.EarnTasks = out
	return r.saveLocked()
}

func (r *Repository) IssueEarnChallenge(ctx context.Context, clientID, userID string) (*domain.EarnPublicTask, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil, 0, domain.ErrClientNotFound
	}
	userIdx := -1
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return nil, 0, domain.ErrUserNotFound
	}
	u := &cs.Users[userIdx]
	now := r.now()
	lockLeft := 0
	if u.EarnLockedUntil > now.Unix() {
		lockLeft = int(u.EarnLockedUntil - now.Unix())
	}

	settings := domain.ResolveEarnSettings(cs.EarnSettings)

	// Anti-cheat: same unfinished question across reload / re-login.
	// Drop sticky challenge if its bank/subject was disabled in settings.
	if u.ActiveEarnChallenge != nil && u.ActiveEarnChallenge.ID != "" {
		if settings.ChallengeAllowed(*u.ActiveEarnChallenge) {
			// Keep prompt sticky, but refresh reward from live settings.
			u.ActiveEarnChallenge.RewardMinutes = settings.RewardForSubject(u.ActiveEarnChallenge.Subject)
			pub := u.ActiveEarnChallenge.Public()
			return &pub, lockLeft, nil
		}
		u.ActiveEarnChallenge = nil
	}

	ch, ok := r.newChallengeLocked(cs, u, settings, now, "")
	if !ok {
		return nil, lockLeft, nil
	}
	u.ActiveEarnChallenge = cloneChallenge(&ch)
	if err := r.saveLocked(); err != nil {
		return nil, lockLeft, err
	}
	pub := ch.Public()
	return &pub, lockLeft, nil
}

// newChallengeLocked picks a random unsolved bank task (builtin + client) or generates math.
func (r *Repository) newChallengeLocked(cs *clientState, u *domain.User, settings domain.EarnSettings, now time.Time, skipBankID string) (domain.EarnChallenge, bool) {
	solved := map[string]bool{}
	for _, id := range u.SolvedTaskIDs {
		solved[id] = true
	}
	rng := rand.New(rand.NewSource(now.UnixNano()))
	var bank []domain.EarnChallenge
	for _, t := range domain.MergeEarnBanks(cs.EarnTasks) {
		if !t.Enabled || t.ID == skipBankID {
			continue
		}
		if domain.IsEarnBankSubject(t.Subject) && !settings.SubjectBankEnabled(t.Subject) {
			continue
		}
		if solved[t.ID] && !settings.SubjectRepeats(t.Subject) {
			continue
		}
		bank = append(bank, domain.EarnChallenge{
			ID:            t.ID,
			Prompt:        t.Prompt,
			Answer:        t.Answer,
			Choices:       domain.ShuffleStrings(rng, t.Choices),
			Kind:          domain.TaskKind(t),
			Subject:       domain.SubjectLabel(t.Subject),
			RewardMinutes: domain.EffectiveReward(t, settings),
			Source:        domain.EarnSourceBank,
			BankTaskID:    t.ID,
			CreatedAt:     now,
		})
	}
	mathOK := settings.MathGeneratorEnabled
	if len(bank) == 0 && !mathOK {
		return domain.EarnChallenge{}, false
	}
	if len(bank) == 0 {
		ch := domain.GenerateMathGrade3(rng, settings.RewardForSubject(domain.SubjectMath))
		ch.ID = "gen-" + uuid.New().String()
		ch.CreatedAt = now
		ch.Choices = domain.ShuffleStrings(rng, ch.Choices)
		return ch, true
	}
	if !mathOK {
		return bank[rng.Intn(len(bank))], true
	}
	// Equal weight: any bank task or a fresh math problem.
	slot := rng.Intn(len(bank) + 1)
	if slot == len(bank) {
		ch := domain.GenerateMathGrade3(rng, settings.RewardForSubject(domain.SubjectMath))
		ch.ID = "gen-" + uuid.New().String()
		ch.CreatedAt = now
		ch.Choices = domain.ShuffleStrings(rng, ch.Choices)
		return ch, true
	}
	return bank[slot], true
}

func (r *Repository) AnswerEarnTask(ctx context.Context, clientID, userID, taskID, answer string) (domain.EarnAnswerResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.EarnAnswerResult{}, domain.ErrClientNotFound
	}
	userIdx := -1
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return domain.EarnAnswerResult{}, domain.ErrUserNotFound
	}
	u := &cs.Users[userIdx]
	now := r.now()
	settings := domain.ResolveEarnSettings(cs.EarnSettings)

	if u.EarnLockedUntil > now.Unix() {
		left := int(u.EarnLockedUntil - now.Unix())
		wrong := 0
		if u.ActiveEarnChallenge != nil {
			wrong = u.ActiveEarnChallenge.WrongCount
		}
		return domain.EarnAnswerResult{
			Correct:        false,
			Balance:        u.EarnBalanceMinutes,
			LockSeconds:    left,
			WrongCount:     wrong,
			WrongStreakMax: settings.WrongStreakLimit,
		}, domain.ErrEarnLocked
	}

	if u.ActiveEarnChallenge == nil || u.ActiveEarnChallenge.ID != taskID {
		return domain.EarnAnswerResult{Balance: u.EarnBalanceMinutes}, domain.ErrTaskNotFound
	}
	ch := u.ActiveEarnChallenge
	reward := settings.RewardForSubject(ch.Subject)
	if reward <= 0 {
		reward = 1
	}

	if !answersMatch(ch.Answer, answer) {
		ch.WrongCount++
		lockSec := settings.WrongLockSeconds
		u.EarnLockedUntil = now.Add(time.Duration(lockSec) * time.Second).Unix()
		subjPenalty := settings.WrongPenaltyForSubject(ch.Subject)
		if subjPenalty > 0 {
			if u.EarnBalanceMinutes <= subjPenalty {
				u.EarnBalanceMinutes = 0
			} else {
				u.EarnBalanceMinutes -= subjPenalty
			}
		}
		result := domain.EarnAnswerResult{
			Correct:        false,
			Balance:        u.EarnBalanceMinutes,
			LockSeconds:    lockSec,
			WrongCount:     ch.WrongCount,
			WrongStreakMax: settings.WrongStreakLimit,
			PenaltyMinutes: subjPenalty,
		}
		wrongDetail := "Неверный ответ"
		if subjPenalty > 0 {
			wrongDetail = fmt.Sprintf("Неверный ответ (−%d мин)", subjPenalty)
		}
		r.appendEarnLogLocked(cs, domain.EarnLogEntry{
			UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogAnswerWrong,
			MinutesDelta: -subjPenalty, BalanceAfter: u.EarnBalanceMinutes,
			TaskID: ch.ID, Subject: ch.Subject, Prompt: ch.Prompt,
			Answer: ch.Answer, GivenAnswer: answer,
			Detail: wrongDetail,
		})
		if ch.WrongCount >= settings.WrongStreakLimit {
			streakPenalty := settings.WrongStreakPenaltyMinutes
			// Per-section wrong penalty already applied each time — don't stack streak minutes.
			if settings.HasSubjectWrongPenalty(ch.Subject) {
				streakPenalty = 0
			}
			if streakPenalty > 0 {
				if u.EarnBalanceMinutes <= streakPenalty {
					u.EarnBalanceMinutes = 0
				} else {
					u.EarnBalanceMinutes -= streakPenalty
				}
				result.PenaltyMinutes += streakPenalty
			}
			skipBank := ch.BankTaskID
			u.ActiveEarnChallenge = nil
			if next, ok := r.newChallengeLocked(cs, u, settings, now, skipBank); ok {
				u.ActiveEarnChallenge = cloneChallenge(&next)
			}
			result.Balance = u.EarnBalanceMinutes
			result.ReplaceQuestion = true
			r.appendEarnLogLocked(cs, domain.EarnLogEntry{
				UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogPenalty,
				MinutesDelta: -streakPenalty, BalanceAfter: u.EarnBalanceMinutes,
				TaskID: ch.ID, Subject: ch.Subject, Prompt: ch.Prompt,
				Detail: "Серия ошибок — новая задача",
			})
		}
		if err := r.saveLocked(); err != nil {
			return domain.EarnAnswerResult{}, err
		}
		return result, nil
	}

	today := now.Format("2006-01-02")
	if u.EarnDayStats.Date != today {
		u.EarnDayStats = domain.EarnDayStats{Date: today}
	}

	newBal, credited := domain.CreditTowardBalanceCap(u.EarnBalanceMinutes, reward, settings.MaxBalanceMinutes)
	u.EarnBalanceMinutes = newBal
	u.EarnDayStats.EarnedMinutes += credited
	u.EarnDayStats.SolvedCount++
	u.EarnLockedUntil = 0
	if ch.Source == domain.EarnSourceBank && ch.BankTaskID != "" && !settings.SubjectRepeats(ch.Subject) {
		already := false
		for _, id := range u.SolvedTaskIDs {
			if id == ch.BankTaskID {
				already = true
				break
			}
		}
		if !already {
			u.SolvedTaskIDs = append(u.SolvedTaskIDs, ch.BankTaskID)
		}
	}
	u.ActiveEarnChallenge = nil
	detail := "Верный ответ"
	if credited == 0 {
		detail = fmt.Sprintf("Верный ответ, но баланс заполнен (макс %d мин)", settings.MaxBalanceMinutes)
	} else if credited < reward {
		detail = fmt.Sprintf("Верный ответ, начислено %d из %d мин (лимит баланса %d)", credited, reward, settings.MaxBalanceMinutes)
	}
	r.appendEarnLogLocked(cs, domain.EarnLogEntry{
		UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogAnswerCorrect,
		MinutesDelta: credited, BalanceAfter: u.EarnBalanceMinutes,
		TaskID: ch.ID, Subject: ch.Subject, Prompt: ch.Prompt,
		Answer: ch.Answer, GivenAnswer: answer,
		Detail: detail,
	})
	if err := r.saveLocked(); err != nil {
		return domain.EarnAnswerResult{}, err
	}
	return domain.EarnAnswerResult{
		Correct:       true,
		Balance:       u.EarnBalanceMinutes,
		RewardMinutes: credited,
	}, nil
}

func (r *Repository) SkipEarnChallenge(ctx context.Context, clientID, userID, taskID string) (domain.EarnAnswerResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.EarnAnswerResult{}, domain.ErrClientNotFound
	}
	userIdx := -1
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return domain.EarnAnswerResult{}, domain.ErrUserNotFound
	}
	u := &cs.Users[userIdx]
	now := r.now()
	settings := domain.ResolveEarnSettings(cs.EarnSettings)

	if u.EarnLockedUntil > now.Unix() {
		left := int(u.EarnLockedUntil - now.Unix())
		wrong := 0
		if u.ActiveEarnChallenge != nil {
			wrong = u.ActiveEarnChallenge.WrongCount
		}
		return domain.EarnAnswerResult{
			Correct:        false,
			Balance:        u.EarnBalanceMinutes,
			LockSeconds:    left,
			WrongCount:     wrong,
			WrongStreakMax: settings.WrongStreakLimit,
		}, domain.ErrEarnLocked
	}

	if u.ActiveEarnChallenge == nil || u.ActiveEarnChallenge.ID != taskID {
		return domain.EarnAnswerResult{Balance: u.EarnBalanceMinutes}, domain.ErrTaskNotFound
	}
	ch := u.ActiveEarnChallenge
	wrong := ch.WrongCount
	skipBank := ch.BankTaskID
	if skipBank == "" {
		skipBank = ch.ID
	}

	penalty := settings.WrongStreakPenaltyMinutes
	if settings.HasSubjectWrongPenalty(ch.Subject) {
		penalty = settings.WrongPenaltyForSubject(ch.Subject)
	}
	if penalty > 0 {
		if u.EarnBalanceMinutes <= penalty {
			u.EarnBalanceMinutes = 0
		} else {
			u.EarnBalanceMinutes -= penalty
		}
	}

	lockSec := settings.WrongLockSeconds
	u.EarnLockedUntil = now.Add(time.Duration(lockSec) * time.Second).Unix()
	u.ActiveEarnChallenge = nil
	if next, ok := r.newChallengeLocked(cs, u, settings, now, skipBank); ok {
		u.ActiveEarnChallenge = cloneChallenge(&next)
	}
	r.appendEarnLogLocked(cs, domain.EarnLogEntry{
		UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogSkip,
		MinutesDelta: -penalty, BalanceAfter: u.EarnBalanceMinutes,
		TaskID: ch.ID, Subject: ch.Subject, Prompt: ch.Prompt,
		Answer: ch.Answer,
		Detail: "Пропуск задачи",
	})
	if err := r.saveLocked(); err != nil {
		return domain.EarnAnswerResult{}, err
	}
	return domain.EarnAnswerResult{
		Correct:         false,
		Balance:         u.EarnBalanceMinutes,
		LockSeconds:     lockSec,
		WrongCount:      wrong,
		WrongStreakMax:  settings.WrongStreakLimit,
		PenaltyMinutes:  penalty,
		ReplaceQuestion: true,
	}, nil
}

func cloneChallenge(c *domain.EarnChallenge) *domain.EarnChallenge {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Choices = append([]string(nil), c.Choices...)
	return &cp
}

func (r *Repository) RedeemEarnMinutes(ctx context.Context, clientID, userID string, minutes int) (domain.EarnRedeemResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if minutes <= 0 {
		return domain.EarnRedeemResult{}, domain.ErrInvalidMinutes
	}
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.EarnRedeemResult{}, domain.ErrClientNotFound
	}
	settings := domain.ResolveEarnSettings(cs.EarnSettings)
	if settings.DisableRedeem {
		return domain.EarnRedeemResult{}, domain.ErrRedeemDisabled
	}
	now := r.now()
	if err := domain.ValidateRedeemMinutes(now, minutes, settings.RedeemSchedule); err != nil {
		return domain.EarnRedeemResult{}, err
	}
	userIdx := -1
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return domain.EarnRedeemResult{}, domain.ErrUserNotFound
	}
	u := &cs.Users[userIdx]
	if u.EarnBalanceMinutes < minutes {
		return domain.EarnRedeemResult{}, domain.ErrInsufficientBalance
	}
	today := now.Format("2006-01-02")
	if u.EarnDayStats.Date != today {
		u.EarnDayStats = domain.EarnDayStats{Date: today}
	}
	spendCap := settings.MaxEarnPerDay
	spent := u.EarnDayStats.SpentMinutes
	if spent+minutes > spendCap {
		return domain.EarnRedeemResult{}, domain.DailySpendLimitError(spent, spendCap, minutes)
	}
	u.EarnBalanceMinutes -= minutes
	u.EarnDayStats.SpentMinutes += minutes
	until := now.Add(time.Duration(minutes) * time.Minute)
	av := domain.RedeemAvailabilityAt(now, settings.RedeemSchedule)
	cs.TemporaryAccessRequests = append(cs.TemporaryAccessRequests, port.TemporaryAccessRequest{
		ID:     uuid.New().String(),
		UserID: userID,
		Start:  now,
		Until:  until,
		Source: domain.TempAccessSourceEarn,
	})
	if len(cs.TemporaryAccessRequests) > maxRequests {
		cs.TemporaryAccessRequests = cs.TemporaryAccessRequests[len(cs.TemporaryAccessRequests)-maxRequests:]
	}
	cs.LastSentVersion = uuid.New().String()
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(r.now(), state, true)
	cs.ComputedConfig = &config
	msg := fmt.Sprintf("Куплено %d мин — войди в свой аккаунт (до %s)", minutes, until.Format("15:04"))
	r.appendEarnLogLocked(cs, domain.EarnLogEntry{
		UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogRedeem,
		MinutesDelta: -minutes, BalanceAfter: u.EarnBalanceMinutes,
		Detail: msg,
	})
	r.notify(clientID)
	if err := r.saveLocked(); err != nil {
		return domain.EarnRedeemResult{}, err
	}
	spendLeft := spendCap - u.EarnDayStats.SpentMinutes
	if spendLeft < 0 {
		spendLeft = 0
	}
	maxAllowed := av.MaxMinutes
	if spendLeft < maxAllowed {
		maxAllowed = spendLeft
	}
	return domain.EarnRedeemResult{
		Minutes:    minutes,
		Until:      until,
		Balance:    u.EarnBalanceMinutes,
		Message:    msg,
		MaxAllowed: maxAllowed,
	}, nil
}

func (r *Repository) RefundEarnSession(ctx context.Context, clientID, userID string) (domain.EarnRefundResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.EarnRefundResult{}, domain.ErrClientNotFound
	}
	userIdx := -1
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return domain.EarnRefundResult{}, domain.ErrUserNotFound
	}
	u := &cs.Users[userIdx]
	now := r.now()
	var maxUntil time.Time
	kept := make([]port.TemporaryAccessRequest, 0, len(cs.TemporaryAccessRequests))
	for _, t := range cs.TemporaryAccessRequests {
		if t.UserID != userID || t.Source != domain.TempAccessSourceEarn || !t.Until.After(now) {
			kept = append(kept, t)
			continue
		}
		if t.Until.After(maxUntil) {
			maxUntil = t.Until
		}
		// Drop active earn sessions (ended early).
	}
	if !maxUntil.After(now) {
		return domain.EarnRefundResult{}, domain.ErrNoEarnSession
	}
	remaining := int(maxUntil.Sub(now) / time.Minute)
	if remaining < 1 {
		return domain.EarnRefundResult{}, domain.ErrNoEarnSession
	}
	cs.TemporaryAccessRequests = kept
	settings := domain.ResolveEarnSettings(cs.EarnSettings)
	newBal, credited := domain.CreditTowardBalanceCap(u.EarnBalanceMinutes, remaining, settings.MaxBalanceMinutes)
	u.EarnBalanceMinutes = newBal
	today := now.Format("2006-01-02")
	if u.EarnDayStats.Date != today {
		u.EarnDayStats = domain.EarnDayStats{Date: today}
	}
	u.EarnDayStats.SpentMinutes -= remaining
	if u.EarnDayStats.SpentMinutes < 0 {
		u.EarnDayStats.SpentMinutes = 0
	}
	cs.LastSentVersion = uuid.New().String()
	state := r.toPortState(cs)
	config, _ := server.ComputeClientConfig(now, state, true)
	cs.ComputedConfig = &config
	msg := fmt.Sprintf("Сессия завершена раньше: возвращено %d мин на баланс", credited)
	if credited < remaining {
		msg = fmt.Sprintf("Сессия завершена раньше: возвращено %d из %d мин (лимит баланса %d)", credited, remaining, settings.MaxBalanceMinutes)
	}
	r.appendEarnLogLocked(cs, domain.EarnLogEntry{
		UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogRefund,
		MinutesDelta: credited, BalanceAfter: u.EarnBalanceMinutes,
		Detail: msg,
	})
	r.notify(clientID)
	if err := r.saveLocked(); err != nil {
		return domain.EarnRefundResult{}, err
	}
	return domain.EarnRefundResult{
		RefundedMinutes: credited,
		Balance:         u.EarnBalanceMinutes,
		Message:         msg,
		EndedAt:         now,
	}, nil
}

func activeEarnAccessLocked(cs *clientState, userID string, now time.Time) *domain.ActiveEarnAccess {
	var maxUntil time.Time
	for _, t := range cs.TemporaryAccessRequests {
		if t.UserID != userID || t.Source != domain.TempAccessSourceEarn {
			continue
		}
		if t.Until.After(now) && t.Until.After(maxUntil) {
			maxUntil = t.Until
		}
	}
	if !maxUntil.After(now) {
		return nil
	}
	rem := int(maxUntil.Sub(now) / time.Minute)
	if rem < 1 {
		return nil
	}
	return &domain.ActiveEarnAccess{RemainingMinutes: rem, Until: maxUntil}
}

func (r *Repository) ClearEarnBalances(ctx context.Context, clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return domain.ErrClientNotFound
	}
	for i := range cs.Users {
		before := cs.Users[i].EarnBalanceMinutes
		cs.Users[i].EarnBalanceMinutes = 0
		cs.Users[i].EarnDayStats = domain.EarnDayStats{}
		if before != 0 {
			r.appendEarnLogLocked(cs, domain.EarnLogEntry{
				UserID: cs.Users[i].ID, UserName: cs.Users[i].Name,
				Kind: domain.EarnLogClearBalance, MinutesDelta: -before, BalanceAfter: 0,
				Detail: "Сброс баланса администратором",
			})
		}
	}
	return r.saveLocked()
}

func (r *Repository) AdjustEarnBalance(ctx context.Context, clientID, userID string, delta int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if delta == 0 {
		return 0, domain.ErrInvalidMinutes
	}
	cs, ok := r.clients[clientID]
	if !ok {
		return 0, domain.ErrClientNotFound
	}
	userIdx := -1
	for i := range cs.Users {
		if cs.Users[i].ID == userID {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		return 0, domain.ErrUserNotFound
	}
	u := &cs.Users[userIdx]
	before := u.EarnBalanceMinutes
	u.EarnBalanceMinutes += delta
	if u.EarnBalanceMinutes < 0 {
		u.EarnBalanceMinutes = 0
	}
	applied := u.EarnBalanceMinutes - before
	detail := fmt.Sprintf("Админ: %+d мин", applied)
	if applied > 0 {
		detail = fmt.Sprintf("Админ добавил %d мин", applied)
	} else if applied < 0 {
		detail = fmt.Sprintf("Админ убавил %d мин", -applied)
	}
	r.appendEarnLogLocked(cs, domain.EarnLogEntry{
		UserID: u.ID, UserName: u.Name, Kind: domain.EarnLogAdjustBalance,
		MinutesDelta: applied, BalanceAfter: u.EarnBalanceMinutes,
		Detail: detail,
	})
	if err := r.saveLocked(); err != nil {
		return 0, err
	}
	return u.EarnBalanceMinutes, nil
}

func (r *Repository) ListEarnLog(ctx context.Context, clientID string, limit int) ([]domain.EarnLogEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cs, ok := r.clients[clientID]
	if !ok {
		return nil, domain.ErrClientNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	n := len(cs.EarnLog)
	if n == 0 {
		return []domain.EarnLogEntry{}, nil
	}
	// Newest first (append order is oldest→newest).
	out := make([]domain.EarnLogEntry, 0, limit)
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, cs.EarnLog[i])
	}
	return out, nil
}

func (r *Repository) appendEarnLogLocked(cs *clientState, e domain.EarnLogEntry) {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.At.IsZero() {
		e.At = r.now()
	}
	cs.EarnLog = append(cs.EarnLog, e)
	if len(cs.EarnLog) > domain.MaxEarnLogEntries {
		cs.EarnLog = cs.EarnLog[len(cs.EarnLog)-domain.MaxEarnLogEntries:]
	}
}

func answersMatch(expected, got string) bool {
	return strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(got))
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
	config, _ := server.ComputeClientConfig(r.now(), state, true)
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
