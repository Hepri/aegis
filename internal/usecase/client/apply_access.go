package client

import (
	"crypto/rand"
	"log"
	"time"

	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

const (
	unlockPassword  = "123456"
	lockPasswordLen = 20
)

// ApplyAccessIfNeeded applies config only when required state differs from lastState.
// lastState: username -> true=allowed, false=blocked. Pass nil on first call.
// Returns the new state after applying.
func ApplyAccessIfNeeded(ctrl port.UserControl, config *domain.ClientConfig, now time.Time, lastState map[string]bool) map[string]bool {
	if len(config.Users) == 0 {
		return lastState
	}
	if lastState == nil {
		lastState = make(map[string]bool)
	}
	newState := make(map[string]bool)

	var changed []string
	for _, uc := range config.Users {
		required := isWithinIntervals(now, uc.AllowedIntervals)
		current := lastState[uc.Username]
		newState[uc.Username] = required

		if required == current {
			// State unchanged, skip
			continue
		}
		changed = append(changed, uc.Username)

		if required {
			if err := ctrl.SetPassword(uc.Username, unlockPassword); err != nil {
				log.Printf("  %s: FAILED to unlock: %v", uc.Username, err)
				newState[uc.Username] = false // keep as blocked on failure
				continue
			}
			log.Printf("  %s: UNLOCKED (was blocked, now in allowed interval)", uc.Username)
		} else {
			randomPass := generateRandomPassword(lockPasswordLen)
			if err := ctrl.SetPassword(uc.Username, randomPass); err != nil {
				log.Printf("  %s: FAILED to block: %v", uc.Username, err)
				newState[uc.Username] = true // keep as allowed on failure
				continue
			}
			log.Printf("  %s: BLOCKED (was allowed, now outside interval)", uc.Username)
			if err := ctrl.DisconnectUserSession(uc.Username); err != nil {
				log.Printf("  %s: session disconnect failed: %v", uc.Username, err)
			}
		}
	}

	if len(changed) > 0 {
		log.Printf("Apply access: changed %v, now=%s", changed, now.Format("15:04 02.01.2006"))
	}
	return newState
}

func isWithinIntervals(t time.Time, intervals []domain.AllowedInterval) bool {
	for _, iv := range intervals {
		if (t.Equal(iv.Start) || t.After(iv.Start)) && t.Before(iv.End) {
			return true
		}
	}
	return false
}

func generateRandomPassword(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}
