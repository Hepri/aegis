package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

func (h *Handler) registerEarnRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /earn", h.ServeEarnPage)
	mux.HandleFunc("GET /api/earn/state", h.EarnState)
	mux.HandleFunc("GET /api/earn/next", h.EarnNext)
	mux.HandleFunc("POST /api/earn/answer", h.EarnAnswer)
	mux.HandleFunc("POST /api/earn/redeem", h.EarnRedeem)
	mux.HandleFunc("PUT /api/clients/{id}/earn-settings", h.UpdateEarnSettings)
}

func (h *Handler) ServeEarnPage(w http.ResponseWriter, r *http.Request) {
	data, err := webFS.ReadFile("web/earn/index.html")
	if err != nil {
		http.Error(w, "earn UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (h *Handler) EarnState(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	userID := r.URL.Query().Get("user_id")
	if clientID == "" {
		http.Error(w, "client_id required", http.StatusBadRequest)
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
	settings := state.EarnSettings
	if settings.DefaultRewardMinutes == 0 && settings.MaxEarnPerDay == 0 {
		settings = domain.DefaultEarnSettings()
	}

	type userInfo struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		BalanceMinutes int    `json:"balance_minutes"`
		AvailableTasks int    `json:"available_tasks"`
		EarnedToday    int    `json:"earned_today"`
	}

	today := time.Now().In(h.loc).Format("2006-01-02")
	users := make([]userInfo, 0, len(state.Users))
	var selected *userInfo
	for _, u := range state.Users {
		solved := map[string]bool{}
		for _, id := range u.SolvedTaskIDs {
			solved[id] = true
		}
		available := 0
		for _, t := range state.EarnTasks {
			if t.Enabled && !solved[t.ID] {
				available++
			}
		}
		earnedToday := 0
		if u.EarnDayStats.Date == today {
			earnedToday = u.EarnDayStats.EarnedMinutes
		}
		info := userInfo{
			ID:             u.ID,
			Name:           u.Name,
			BalanceMinutes: u.EarnBalanceMinutes,
			AvailableTasks: available,
			EarnedToday:    earnedToday,
		}
		users = append(users, info)
		if userID != "" && u.ID == userID {
			cp := info
			selected = &cp
		}
	}

	resp := map[string]any{
		"client_id":      state.ID,
		"client_name":    state.Name,
		"task_count":     len(state.EarnTasks),
		"enabled_tasks":  countEnabledTasks(state.EarnTasks),
		"earn_settings":  settings,
		"users":          users,
		"redeem_options": []int{5, 15, 30},
	}
	if selected != nil {
		resp["user"] = selected
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func countEnabledTasks(tasks []domain.EarnTask) int {
	n := 0
	for _, t := range tasks {
		if t.Enabled {
			n++
		}
	}
	return n
}

func (h *Handler) EarnNext(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	userID := r.URL.Query().Get("user_id")
	if clientID == "" || userID == "" {
		http.Error(w, "client_id and user_id required", http.StatusBadRequest)
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
	var user *domain.User
	for i := range state.Users {
		if state.Users[i].ID == userID {
			user = &state.Users[i]
			break
		}
	}
	if user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	solved := map[string]bool{}
	for _, id := range user.SolvedTaskIDs {
		solved[id] = true
	}
	for _, t := range state.EarnTasks {
		if !t.Enabled || solved[t.ID] {
			continue
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":             t.ID,
			"prompt":         t.Prompt,
			"reward_minutes": domain.EffectiveReward(t, state.EarnSettings),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"empty": true})
}

func (h *Handler) EarnAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		UserID   string `json:"user_id"`
		TaskID   string `json:"task_id"`
		Answer   string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ClientID == "" || req.UserID == "" || req.TaskID == "" {
		http.Error(w, "client_id, user_id, task_id required", http.StatusBadRequest)
		return
	}
	correct, balance, err := h.repo.AnswerEarnTask(r.Context(), req.ClientID, req.UserID, req.TaskID, req.Answer)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound), errors.Is(err, domain.ErrTaskNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrTaskAlreadySolved), errors.Is(err, domain.ErrDailyLimit):
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"correct":         correct,
		"balance_minutes": balance,
	})
}

func (h *Handler) EarnRedeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		UserID   string `json:"user_id"`
		Minutes  int    `json:"minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.repo.RedeemEarnMinutes(r.Context(), req.ClientID, req.UserID, req.Minutes); err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrInsufficientBalance):
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	state, _ := h.repo.GetClient(r.Context(), req.ClientID)
	balance := 0
	if state != nil {
		for _, u := range state.Users {
			if u.ID == req.UserID {
				balance = u.EarnBalanceMinutes
				break
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":              true,
		"minutes":         req.Minutes,
		"balance_minutes": balance,
		"message":         "Время куплено — войди в свой аккаунт",
	})
}

func (h *Handler) UpdateEarnSettings(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	var req domain.EarnSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.DefaultRewardMinutes < 0 || req.MaxEarnPerDay < 0 {
		http.Error(w, "values must be non-negative", http.StatusBadRequest)
		return
	}
	if req.DefaultRewardMinutes == 0 {
		req.DefaultRewardMinutes = domain.DefaultEarnSettings().DefaultRewardMinutes
	}
	if req.MaxEarnPerDay == 0 {
		req.MaxEarnPerDay = domain.DefaultEarnSettings().MaxEarnPerDay
	}
	if err := h.repo.UpdateEarnSettings(r.Context(), clientID, req); err != nil {
		if errors.Is(err, domain.ErrClientNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
