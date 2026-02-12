package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
	"github.com/aegis/parental-control/internal/usecase/server"
)

const (
	longPollTimeout     = 60 * time.Second
	maxLongPollInterval = 55 * time.Second
)

type Handler struct {
	repo port.ConfigRepository
}

func NewHandler(repo port.ConfigRepository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) ServeConfig(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "client_id required", http.StatusBadRequest)
		return
	}
	clientVersion := r.URL.Query().Get("version")

	ctx := r.Context()

	// Get client state (must exist, no auto-registration)
	state, err := h.repo.GetClient(ctx, clientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state == nil {
		http.Error(w, "client not found", http.StatusForbidden)
		return
	}

	// Get precomputed config (always today+tomorrow, full)
	if state.ComputedConfig == nil {
		http.Error(w, "config not computed", http.StatusInternalServerError)
		return
	}
	config := *state.ComputedConfig

	// Compute next change time (when to wake up from long poll)
	now := time.Now()
	_, nextChange := server.ComputeClientConfig(now, state, true)

	// Check if config version changed (admin made changes)
	versionChanged := state.LastSentVersion != config.Version

	// If client sent version and it differs — respond immediately
	if clientVersion != "" && clientVersion != config.Version {
		h.sendConfig(w, r, config, clientID)
		return
	}

	// If version changed (admin made changes) — respond immediately
	if versionChanged {
		h.sendConfig(w, r, config, clientID)
		return
	}

	// If client didn't send version — respond immediately (for old clients / curl)
	if clientVersion == "" {
		h.sendConfig(w, r, config, clientID)
		return
	}

	// Wait for changes
	subCh := h.repo.Subscribe(ctx, clientID)
	deadline := time.Now().Add(longPollTimeout)
	if nextChange.Before(deadline) {
		deadline = nextChange
	}
	if deadline.Sub(now) > maxLongPollInterval {
		deadline = now.Add(maxLongPollInterval)
	}

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	select {
	case <-subCh:
		// Config changed, get updated precomputed config
		state, _ = h.repo.GetClient(ctx, clientID)
		if state != nil && state.ComputedConfig != nil {
			newConfig := *state.ComputedConfig
			if newConfig.Version != config.Version {
				h.sendConfig(w, r, newConfig, clientID)
				return
			}
		}
		// Fall through to timeout
	case <-timer.C:
		// Timeout - check if version changed
		state, _ = h.repo.GetClient(ctx, clientID)
		if state != nil && state.ComputedConfig != nil {
			newConfig := *state.ComputedConfig
			if newConfig.Version != config.Version {
				h.sendConfig(w, r, newConfig, clientID)
				return
			}
		}
		// Nothing changed - close connection, client will reconnect
	case <-r.Context().Done():
		return
	}
}

func (h *Handler) sendConfig(w http.ResponseWriter, r *http.Request, config domain.ClientConfig, clientID string) {
	ctx := r.Context()
	state, _ := h.repo.GetClient(ctx, clientID)
	if state != nil {
		intervals := make(map[string][]domain.AllowedInterval)
		for _, uc := range config.Users {
			intervals[uc.Username] = uc.AllowedIntervals
		}
		h.repo.UpdateLastSent(ctx, clientID, intervals)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}
