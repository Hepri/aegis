package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
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
}

func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.repo.GetAllClients(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type clientInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	result := make([]clientInfo, 0, len(clients))
	for _, c := range clients {
		result = append(result, clientInfo{ID: c.ID, Name: c.Name})
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
	}{
		ID:                      state.ID,
		Name:                    state.Name,
		BlockRequests:           state.BlockRequests,
		TemporaryAccessRequests: state.TemporaryAccessRequests,
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
