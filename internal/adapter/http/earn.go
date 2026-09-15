package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

func (h *Handler) registerEarnRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /earn", h.ServeEarnPage)
	mux.HandleFunc("GET /earn/bank", h.ServeEarnBankPage)
	mux.HandleFunc("GET /api/earn/state", h.EarnState)
	mux.HandleFunc("GET /api/earn/next", h.EarnNext)
	mux.HandleFunc("POST /api/earn/answer", h.EarnAnswer)
	mux.HandleFunc("POST /api/earn/skip", h.EarnSkip)
	mux.HandleFunc("POST /api/earn/redeem", h.EarnRedeem)
	mux.HandleFunc("POST /api/earn/refund-session", h.EarnRefundSession)
	mux.HandleFunc("POST /api/earn-admin/unlock", h.EarnAdminUnlock)
	mux.HandleFunc("GET /api/earn-admin/bank", h.EarnAdminBank)
	mux.HandleFunc("POST /api/clients/{id}/earn-tasks", h.UpsertEarnTask)
	mux.HandleFunc("DELETE /api/clients/{id}/earn-tasks/{taskId}", h.DeleteEarnTask)
	mux.HandleFunc("GET /api/clients/{id}/earn-log", h.EarnLog)
	mux.HandleFunc("PUT /api/clients/{id}/earn-settings", h.UpdateEarnSettings)
	mux.HandleFunc("POST /api/clients/{id}/earn-balances/clear", h.ClearEarnBalances)
	mux.HandleFunc("POST /api/clients/{id}/users/{userId}/earn-balance", h.AdjustEarnBalance)
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

func (h *Handler) ServeEarnBankPage(w http.ResponseWriter, r *http.Request) {
	data, err := webFS.ReadFile("web/earn/bank.html")
	if err != nil {
		http.Error(w, "earn bank UI not found", http.StatusNotFound)
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
	settings := domain.ResolveEarnSettings(state.EarnSettings)

	type userInfo struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		BalanceMinutes int    `json:"balance_minutes"`
		AvailableTasks int    `json:"available_tasks"`
		EarnedToday    int    `json:"earned_today"`
		SpentToday     int    `json:"spent_today"`
		SpendLimit     int    `json:"spend_limit_today"`
		SpendLeft      int    `json:"spend_left_today"`
		BalanceCap     int    `json:"balance_cap"`
		BalanceRoom    int    `json:"balance_room"`
		LockSeconds    int    `json:"lock_seconds,omitempty"`
		WrongCount     int    `json:"wrong_count,omitempty"`
	}

	nowUnix := time.Now().In(h.loc).Unix()
	today := time.Now().In(h.loc).Format("2006-01-02")
	users := make([]userInfo, 0, len(state.Users))
	var selected *userInfo
	for _, u := range state.Users {
		solved := map[string]bool{}
		for _, id := range u.SolvedTaskIDs {
			solved[id] = true
		}
		available := 0
		for _, t := range domain.MergeEarnBanks(state.EarnTasks) {
			if !t.Enabled {
				continue
			}
			if domain.IsEarnBankSubject(t.Subject) && !settings.SubjectBankEnabled(t.Subject) {
				continue
			}
			if solved[t.ID] && !settings.SubjectRepeats(t.Subject) {
				continue
			}
			available++
		}
		if settings.MathGeneratorEnabled {
			available++ // at least generator
		}
		earnedToday := 0
		spentToday := 0
		if u.EarnDayStats.Date == today {
			earnedToday = u.EarnDayStats.EarnedMinutes
			spentToday = u.EarnDayStats.SpentMinutes
		}
		spendLimit := settings.MaxEarnPerDay
		spendLeft := spendLimit - spentToday
		if spendLeft < 0 {
			spendLeft = 0
		}
		balanceCap := settings.MaxBalanceMinutes
		balanceRoom := balanceCap - u.EarnBalanceMinutes
		if balanceRoom < 0 {
			balanceRoom = 0
		}
		lockSec := 0
		if u.EarnLockedUntil > nowUnix {
			lockSec = int(u.EarnLockedUntil - nowUnix)
		}
		wrong := 0
		if u.ActiveEarnChallenge != nil {
			wrong = u.ActiveEarnChallenge.WrongCount
		}
		info := userInfo{
			ID:             u.ID,
			Name:           u.Name,
			BalanceMinutes: u.EarnBalanceMinutes,
			AvailableTasks: available,
			EarnedToday:    earnedToday,
			SpentToday:     spentToday,
			SpendLimit:     spendLimit,
			SpendLeft:      spendLeft,
			BalanceCap:     balanceCap,
			BalanceRoom:    balanceRoom,
			LockSeconds:    lockSec,
			WrongCount:     wrong,
		}
		users = append(users, info)
		if userID != "" && u.ID == userID {
			cp := info
			selected = &cp
		}
	}

	av := domain.RedeemAvailabilityAt(time.Now().In(h.loc), settings.RedeemSchedule)
	redeemMax := av.MaxMinutes
	redeemAllowed := av.Allowed
	redeemHint := av.Message
	if selected != nil {
		if selected.SpendLeft < redeemMax {
			redeemMax = selected.SpendLeft
		}
		if selected.SpendLeft <= 0 {
			redeemAllowed = false
			redeemMax = 0
			redeemHint = fmt.Sprintf("Дневной лимит траты исчерпан: уже потрачено %d из %d мин", selected.SpentToday, selected.SpendLimit)
		} else if av.Allowed {
			until := av.QuietUntil.Format("15:04")
			redeemHint = fmt.Sprintf("Можно купить до %d мин (сегодня потрачено %d из %d; до %s)",
				redeemMax, selected.SpentToday, selected.SpendLimit, until)
		}
	}
	resp := map[string]any{
		"client_id":          state.ID,
		"client_name":        state.Name,
		"task_count":         len(state.EarnTasks),
		"enabled_tasks":      countEnabledTasks(state.EarnTasks),
		"earn_settings":      settings,
		"redeem_enabled":     !settings.DisableRedeem,
		"users":              users,
		"redeem_options":     []int{5, 15, 30},
		"redeem_max_minutes": redeemMax,
		"redeem_allowed":     redeemAllowed && redeemMax > 0,
		"redeem_hint":        redeemHint,
		"redeem_until":       av.QuietUntil,
	}
	if selected != nil {
		resp["user"] = selected
		if acc := activeEarnAccessFromState(state, selected.ID, time.Now().In(h.loc)); acc != nil {
			resp["active_earn_access"] = acc
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func activeEarnAccessFromState(state *port.ClientState, userID string, now time.Time) *domain.ActiveEarnAccess {
	var maxUntil time.Time
	for _, t := range state.TemporaryAccessRequests {
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
	task, lockSec, err := h.repo.IssueEarnChallenge(r.Context(), clientID, userID)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound):
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if task == nil {
		json.NewEncoder(w).Encode(map[string]any{"empty": true, "lock_seconds": lockSec})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"id":             task.ID,
		"prompt":         task.Prompt,
		"kind":           task.Kind,
		"subject":        task.Subject,
		"choices":        task.Choices,
		"reward_minutes": task.RewardMinutes,
		"source":         task.Source,
		"wrong_count":    task.WrongCount,
		"lock_seconds":   lockSec,
	})
}

func (h *Handler) writeEarnError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"message": err.Error()})
}

func (h *Handler) EarnAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		UserID   string `json:"user_id"`
		TaskID   string `json:"task_id"`
		Answer   string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeEarnError(w, fmt.Errorf("неверный запрос"), http.StatusBadRequest)
		return
	}
	if req.ClientID == "" || req.UserID == "" || req.TaskID == "" {
		h.writeEarnError(w, fmt.Errorf("нужны client_id, user_id и task_id"), http.StatusBadRequest)
		return
	}
	result, err := h.repo.AnswerEarnTask(r.Context(), req.ClientID, req.UserID, req.TaskID, req.Answer)
	if err != nil {
		if errors.Is(err, domain.ErrEarnLocked) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			json.NewEncoder(w).Encode(result)
			return
		}
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound), errors.Is(err, domain.ErrTaskNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrTaskAlreadySolved), errors.Is(err, domain.ErrDailyLimit):
			status = http.StatusConflict
		}
		h.writeEarnError(w, err, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) EarnSkip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		UserID   string `json:"user_id"`
		TaskID   string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ClientID == "" || req.UserID == "" || req.TaskID == "" {
		http.Error(w, "client_id, user_id, task_id required", http.StatusBadRequest)
		return
	}
	result, err := h.repo.SkipEarnChallenge(r.Context(), req.ClientID, req.UserID, req.TaskID)
	if err != nil {
		if errors.Is(err, domain.ErrEarnLocked) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			json.NewEncoder(w).Encode(result)
			return
		}
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound), errors.Is(err, domain.ErrTaskNotFound):
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) EarnRedeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		UserID   string `json:"user_id"`
		Minutes  int    `json:"minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeEarnError(w, fmt.Errorf("неверный запрос"), http.StatusBadRequest)
		return
	}
	result, err := h.repo.RedeemEarnMinutes(r.Context(), req.ClientID, req.UserID, req.Minutes)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrInsufficientBalance), errors.Is(err, domain.ErrDailyLimit):
			status = http.StatusConflict
		case errors.Is(err, domain.ErrRedeemDisabled):
			status = http.StatusForbidden
		case errors.Is(err, domain.ErrRedeemCurfew), errors.Is(err, domain.ErrRedeemTooLong):
			status = http.StatusConflict
		}
		h.writeEarnError(w, err, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":                  true,
		"minutes":             result.Minutes,
		"balance_minutes":     result.Balance,
		"until":               result.Until,
		"message":             result.Message,
		"max_allowed_minutes": result.MaxAllowed,
	})
}

func (h *Handler) EarnRefundSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		UserID   string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeEarnError(w, fmt.Errorf("неверный запрос"), http.StatusBadRequest)
		return
	}
	if req.ClientID == "" || req.UserID == "" {
		h.writeEarnError(w, fmt.Errorf("нужны client_id и user_id"), http.StatusBadRequest)
		return
	}
	result, err := h.repo.RefundEarnSession(r.Context(), req.ClientID, req.UserID)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrNoEarnSession):
			status = http.StatusConflict
		}
		h.writeEarnError(w, err, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":               true,
		"refunded_minutes": result.RefundedMinutes,
		"balance_minutes":  result.Balance,
		"message":          result.Message,
		"ended_at":         result.EndedAt,
	})
}

func (h *Handler) EarnLog(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.PathValue("id")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	entries, err := h.repo.ListEarnLog(r.Context(), clientID, limit)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, domain.ErrClientNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"client_id": clientID,
		"entries":   entries,
	})
}

func (h *Handler) EarnAdminUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !h.earnAdminPasswordMatches(req.Password) {
		http.Error(w, "неверный пароль", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *Handler) EarnAdminBank(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.URL.Query().Get("client_id")
	var clientTasks []domain.EarnTask
	if clientID != "" {
		st, err := h.repo.GetClient(r.Context(), clientID)
		if err == nil && st != nil {
			clientTasks = st.EarnTasks
		}
	}
	customIDs := map[string]bool{}
	for _, t := range clientTasks {
		customIDs[t.ID] = true
	}
	tasks := domain.MergeEarnBanks(clientTasks)
	bySubject := map[string]int{}
	customSubjects := map[string]bool{}
	type taskView struct {
		ID            string   `json:"id"`
		Subject       string   `json:"subject"`
		Prompt        string   `json:"prompt"`
		Answer        string   `json:"answer"`
		Choices       []string `json:"choices,omitempty"`
		Kind          string   `json:"kind"`
		RewardMinutes int      `json:"reward_minutes"`
		Source        string   `json:"source"`
		Editable      bool     `json:"editable"`
		Enabled       bool     `json:"enabled"`
	}
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		subj := domain.SubjectLabel(t.Subject)
		bySubject[subj]++
		editable := customIDs[t.ID]
		src := domain.EarnSourceBank
		if editable {
			src = domain.EarnSourceCustom
			if !domain.IsEarnBankSubject(subj) {
				customSubjects[subj] = true
			}
		}
		views = append(views, taskView{
			ID: t.ID, Subject: subj, Prompt: t.Prompt, Answer: t.Answer,
			Choices: append([]string(nil), t.Choices...), Kind: domain.TaskKind(t),
			RewardMinutes: t.RewardMinutes, Source: src, Editable: editable, Enabled: t.Enabled,
		})
	}
	subjects := append([]string{}, domain.EarnBankSubjects()...)
	for s := range customSubjects {
		subjects = append(subjects, s)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"task_count":       len(views),
		"custom_count":     len(clientTasks),
		"by_subject":       bySubject,
		"tasks":            views,
		"math_note":        "Математика: 100 задач в банке + дополнительная генерация новых составных задач.",
		"subjects":         subjects,
		"builtin_subjects": domain.EarnBankSubjects(),
		"client_id":        clientID,
	})
}

func (h *Handler) UpsertEarnTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.PathValue("id")
	var req domain.EarnTask
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := h.repo.UpsertEarnTask(r.Context(), clientID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrBuiltinTask):
			status = http.StatusForbidden
		}
		h.writeEarnError(w, err, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":             task.ID,
		"subject":        task.Subject,
		"prompt":         task.Prompt,
		"answer":         task.Answer,
		"choices":        task.Choices,
		"kind":           domain.TaskKind(task),
		"reward_minutes": task.RewardMinutes,
		"source":         domain.EarnSourceCustom,
		"editable":       true,
		"enabled":        task.Enabled,
	})
}

func (h *Handler) DeleteEarnTask(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.PathValue("id")
	taskID := r.PathValue("taskId")
	if err := h.repo.DeleteEarnTask(r.Context(), clientID, taskID); err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrTaskNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrBuiltinTask):
			status = http.StatusForbidden
		}
		h.writeEarnError(w, err, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *Handler) UpdateEarnSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.PathValue("id")
	var req domain.EarnSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.DefaultRewardMinutes < 0 || req.MaxEarnPerDay < 0 || req.MaxBalanceMinutes < 0 || req.WrongLockSeconds < 0 ||
		req.WrongStreakLimit < 0 || req.WrongStreakPenaltyMinutes < 0 {
		http.Error(w, "values must be non-negative", http.StatusBadRequest)
		return
	}
	gen := req.MathGeneratorEnabled
	disableRedeem := req.DisableRedeem
	req = domain.NormalizeEarnSettings(req)
	req.MathGeneratorEnabled = gen
	req.DisableRedeem = disableRedeem
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

func (h *Handler) ClearEarnBalances(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.PathValue("id")
	if err := h.repo.ClearEarnBalances(r.Context(), clientID); err != nil {
		if errors.Is(err, domain.ErrClientNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *Handler) AdjustEarnBalance(w http.ResponseWriter, r *http.Request) {
	if !h.requireEarnAdmin(w, r) {
		return
	}
	clientID := r.PathValue("id")
	userID := r.PathValue("userId")
	var req struct {
		Delta int `json:"delta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeEarnError(w, fmt.Errorf("неверный запрос"), http.StatusBadRequest)
		return
	}
	if req.Delta == 0 {
		h.writeEarnError(w, domain.ErrInvalidMinutes, http.StatusBadRequest)
		return
	}
	balance, err := h.repo.AdjustEarnBalance(r.Context(), clientID, userID, req.Delta)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, domain.ErrClientNotFound), errors.Is(err, domain.ErrUserNotFound):
			status = http.StatusNotFound
		}
		h.writeEarnError(w, err, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":               true,
		"balance_minutes":  balance,
		"delta":            req.Delta,
	})
}
