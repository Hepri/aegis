package server

import (
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

// ComputeClientConfig computes ClientConfig with allowed_intervals for a client.
// includePast: if true, includes intervals that already ended (for admin preview).
func ComputeClientConfig(now time.Time, state *port.ClientState, includePast bool) (domain.ClientConfig, time.Time) {
	version := state.LastSentVersion
	var users []domain.UserAccessConfig

	tempAccessByUser := make(map[string][]domain.TempAccessRange)
	for _, t := range state.TemporaryAccessRequests {
		tempAccessByUser[t.UserID] = append(tempAccessByUser[t.UserID], domain.TempAccessRange{Start: t.Start, End: t.Until})
	}

	// Separate blocks: global (no UserID) and per-user
	globalBlocks := make([]domain.BlockRange, 0)
	blocksByUser := make(map[string][]domain.BlockRange)
	for _, b := range state.BlockRequests {
		block := domain.BlockRange{Start: b.Start, End: b.Until}
		if b.UserID == "" {
			globalBlocks = append(globalBlocks, block)
		} else {
			blocksByUser[b.UserID] = append(blocksByUser[b.UserID], block)
		}
	}

	for _, u := range state.Users {
		ta := tempAccessByUser[u.ID]
		// Combine global blocks + user-specific blocks
		userBlocks := append([]domain.BlockRange(nil), globalBlocks...)
		userBlocks = append(userBlocks, blocksByUser[u.ID]...)
		intervals, _ := domain.ComputeAllowedIntervals(now, u.Schedule, ta, userBlocks, includePast)
		users = append(users, domain.UserAccessConfig{
			Username:         u.Username,
			AllowedIntervals: intervals,
		})
	}

	// Compute next change time from first user (simplified - take min across all)
	var nextChange time.Time
	for i, u := range state.Users {
		ta := tempAccessByUser[u.ID]
		userBlocks := append([]domain.BlockRange(nil), globalBlocks...)
		userBlocks = append(userBlocks, blocksByUser[u.ID]...)
		_, nc := domain.ComputeAllowedIntervals(now, u.Schedule, ta, userBlocks, includePast)
		if i == 0 || nc.Before(nextChange) {
			nextChange = nc
		}
	}
	if nextChange.IsZero() {
		nextChange = now.Add(48 * time.Hour)
	}

	return domain.ClientConfig{
		Users:   users,
		Version: version,
	}, nextChange
}

// IntervalsChanged returns true if intervals differ from last sent
func IntervalsChanged(state *port.ClientState, newConfig domain.ClientConfig) bool {
	if state.LastSentVersion == "" {
		return true // first connection, always send
	}
	if len(state.LastSentIntervals) != len(newConfig.Users) {
		return true
	}
	for _, uc := range newConfig.Users {
		last := state.LastSentIntervals[uc.Username]
		if !intervalsEqual(last, uc.AllowedIntervals) {
			return true
		}
	}
	// Check if any user was removed
	for username := range state.LastSentIntervals {
		found := false
		for _, uc := range newConfig.Users {
			if uc.Username == username {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// comparePrecision truncates time for comparison to avoid false "changed" when
// interval boundaries are clipped to "now" (which differs each poll).
const comparePrecision = time.Minute

func intervalsEqual(a, b []domain.AllowedInterval) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		as := a[i].Start.Truncate(comparePrecision)
		bs := b[i].Start.Truncate(comparePrecision)
		ae := a[i].End.Truncate(comparePrecision)
		be := b[i].End.Truncate(comparePrecision)
		if !as.Equal(bs) || !ae.Equal(be) {
			return false
		}
	}
	return true
}
