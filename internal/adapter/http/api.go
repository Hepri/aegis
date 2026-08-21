package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
	"github.com/aegis/parental-control/internal/usecase/server"
	"github.com/google/uuid"
)

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", h.ServeConfig)
	mux.HandleFunc("GET /api/clients", h.ListClients)
	mux.HandleFunc("POST /api/clients", h.CreateClient)
	mux.HandleFunc("GET /api/clients/{id}", h.GetClient)
	mux.HandleFunc("GET /api/clients/{id}/preview", h.GetClientPreview)
	mux.HandleFunc("DELETE /api/clients/{id}", h.DeleteClient)
	mux.HandleFunc("POST /api/clients/{id}/users", h.AddUser)
	mux.HandleFunc("PUT /api/clients/{id}/users/{uid}/schedule", h.UpdateSchedule)
	mux.HandleFunc("DELETE /api/clients/{id}/users/{uid}", h.DeleteUser)
	mux.HandleFunc("POST /api/clients/{id}/temporary-access", h.TemporaryAccess)
	mux.HandleFunc("DELETE /api/clients/{id}/temporary-access/{rid}", h.DeleteTemporaryAccess)
	mux.HandleFunc("POST /api/clients/{id}/block", h.Block)
	mux.HandleFunc("DELETE /api/clients/{id}/block/{rid}", h.DeleteBlock)
	mux.HandleFunc("POST /api/clients/{id}/events", h.PostEvents)
	mux.HandleFunc("GET /api/clients/{id}/activity", h.GetActivity)
	mux.HandleFunc("GET /api/updates/client", h.GetUpdateManifest)
	mux.HandleFunc("GET /api/updates/aegis-client.exe", h.DownloadClientBinary)
}

func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.repo.GetAllClients(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type clientInfo struct {
		ID            string     `json:"id"`
		Name          string     `json:"name"`
		LastSeen      *time.Time `json:"last_seen,omitempty"`
		Online        bool       `json:"online"`
		ClientVersion string     `json:"client_version,omitempty"`
	}
	result := make([]clientInfo, 0, len(clients))
	var allPresence map[string]port.ClientPresence
	if h.presence != nil {
		allPresence = h.presence.GetAllPresence(r.Context())
	}
	now := time.Now()
	for _, c := range clients {
		info := clientInfo{ID: c.ID, Name: c.Name}
		if pr, ok := allPresence[c.ID]; ok {
			tt := pr.LastSeen
			info.LastSeen = &tt
			info.Online = now.Sub(pr.LastSeen) < 2*time.Minute
			info.ClientVersion = pr.ClientVersion
		}
		result = append(result, info)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := uuid.New().String()
	state := &port.ClientState{
		ID:                      id,
		Name:                    req.Name,
		Users:                   nil,
		BlockRequests:           nil,
		TemporaryAccessRequests: nil,
	}
	if err := h.repo.SaveClient(r.Context(), state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	if err := h.repo.DeleteClient(r.Context(), clientID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	state, err := h.repo.GetClient(r.Context(), clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	type userResp struct {
		ID       string             `json:"id"`
		Name     string             `json:"name"`
		Username string             `json:"username"`
		Schedule domain.DaySchedule `json:"schedule"`
	}
	resp := struct {
		ID                      string                        `json:"id"`
		Name                    string                        `json:"name"`
		Users                   []userResp                    `json:"users"`
		BlockRequests           []port.BlockRequest           `json:"block_requests"`
		TemporaryAccessRequests []port.TemporaryAccessRequest `json:"temporary_access_requests"`
		LastSeen                *time.Time                    `json:"last_seen,omitempty"`
		Online                  bool                          `json:"online"`
		ClientVersion           string                        `json:"client_version,omitempty"`
	}{
		ID:                      state.ID,
		Name:                    state.Name,
		BlockRequests:           state.BlockRequests,
		TemporaryAccessRequests: state.TemporaryAccessRequests,
	}
	if h.presence != nil {
		if pr, ok := h.presence.GetPresence(r.Context(), clientID); ok {
			tt := pr.LastSeen
			resp.LastSeen = &tt
			resp.Online = time.Now().Sub(pr.LastSeen) < 2*time.Minute
			resp.ClientVersion = pr.ClientVersion
		}
	}
	for _, u := range state.Users {
		resp.Users = append(resp.Users, userResp{
			ID:       u.ID,
			Name:     u.Name,
			Username: u.Username,
			Schedule: u.Schedule,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetClientPreview(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	state, err := h.repo.GetClient(r.Context(), clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if state.ComputedConfig == nil {
		http.Error(w, "config not computed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.ComputedConfig)
}

func (h *Handler) AddUser(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	var req struct {
		Name     string             `json:"name"`
		Username string             `json:"username"`
		Schedule domain.DaySchedule `json:"schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user := domain.User{
		ID:       uuid.New().String(),
		Name:     req.Name,
		Username: req.Username,
		Schedule: req.Schedule,
	}
	if user.Schedule == nil {
		user.Schedule = make(domain.DaySchedule)
	}
	if err := h.repo.AddUser(r.Context(), clientID, user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": user.ID})
}

func (h *Handler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	userID := r.PathValue("uid")
	var req struct {
		Schedule domain.DaySchedule `json:"schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.repo.UpdateUserSchedule(r.Context(), clientID, userID, req.Schedule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	userID := r.PathValue("uid")
	if err := h.repo.DeleteUser(r.Context(), clientID, userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) TemporaryAccess(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	var req struct {
		UserID   string `json:"user_id"`
		Duration int    `json:"duration"` // minutes
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Duration <= 0 {
		http.Error(w, "duration must be positive", http.StatusBadRequest)
		return
	}
	now := time.Now().In(h.loc)
	until := now.Add(time.Duration(req.Duration) * time.Minute)
	if err := h.repo.GrantTemporaryAccess(r.Context(), clientID, req.UserID, until); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Block(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	var req struct {
		UserID   string `json:"user_id,omitempty"` // empty = block all
		Duration int    `json:"duration"`          // minutes
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Duration <= 0 {
		http.Error(w, "duration must be positive", http.StatusBadRequest)
		return
	}
	now := time.Now().In(h.loc)
	until := now.Add(time.Duration(req.Duration) * time.Minute)
	if err := h.repo.BlockClient(r.Context(), clientID, req.UserID, now, until); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	requestID := r.PathValue("rid")
	if err := h.repo.DeleteBlockRequest(r.Context(), clientID, requestID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) DeleteTemporaryAccess(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	requestID := r.PathValue("rid")
	if err := h.repo.DeleteTemporaryAccessRequest(r.Context(), clientID, requestID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.repo.IncrementConfigVersion(r.Context(), clientID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) PostEvents(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	if h.activity == nil {
		http.Error(w, "activity store not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := h.repo.GetClient(r.Context(), clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "client not found", http.StatusForbidden)
		return
	}
	var req struct {
		Events []domain.ActivityEvent `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Events) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	for i := range req.Events {
		if req.Events[i].ID == "" {
			req.Events[i].ID = uuid.New().String()
		}
		if req.Events[i].Timestamp.IsZero() {
			req.Events[i].Timestamp = time.Now().In(h.loc)
		}
	}
	if err := h.activity.AppendEvents(r.Context(), clientID, req.Events); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if h.presence != nil {
		_ = h.presence.TouchPresence(r.Context(), clientID, "")
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	if h.activity == nil {
		http.Error(w, "activity store not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := h.repo.GetClient(r.Context(), clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	dateStr := r.URL.Query().Get("date")
	var day time.Time
	if dateStr == "" {
		day = time.Now().In(h.loc)
	} else {
		day, err = time.ParseInLocation("2006-01-02", dateStr, h.loc)
		if err != nil {
			http.Error(w, "invalid date, want YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	}
	events, err := h.activity.ReadDayEvents(r.Context(), clientID, day)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	agg := server.AggregateDayActivity(day, events, time.Now().In(h.loc))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agg)
}

func (h *Handler) GetUpdateManifest(w http.ResponseWriter, r *http.Request) {
	if h.updates == nil {
		http.Error(w, "updates not configured", http.StatusNotFound)
		return
	}
	u := h.updates.GetClientUpdate()
	if u == nil {
		http.Error(w, "no update available", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u)
}

func (h *Handler) DownloadClientBinary(w http.ResponseWriter, r *http.Request) {
	if h.updates == nil {
		http.Error(w, "updates not configured", http.StatusNotFound)
		return
	}
	path, err := h.updates.BinaryPath()
	if err != nil {
		http.Error(w, "binary not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="aegis-client.exe"`)
	http.ServeFile(w, r, path)
}
