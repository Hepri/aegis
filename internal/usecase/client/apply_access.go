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

// ApplyAccess applies the config to users via UserControl
func ApplyAccess(ctrl port.UserControl, config *domain.ClientConfig, now time.Time) error {
	if len(config.Users) == 0 {
		log.Printf("Apply access: no users in config")
		return nil
	}
	log.Printf("Apply access rules: %d users, now=%s", len(config.Users), now.Format("15:04 02.01.2006"))

	var unlocked, blocked []string
	for _, uc := range config.Users {
		allowed := isWithinIntervals(now, uc.AllowedIntervals)
		if allowed {
			if err := ctrl.SetPassword(uc.Username, unlockPassword); err != nil {
				log.Printf("  %s: FAILED to unlock: %v", uc.Username, err)
				continue
			}
			log.Printf("  %s: UNLOCKED (in allowed interval)", uc.Username)
			unlocked = append(unlocked, uc.Username)
		} else {
			randomPass := generateRandomPassword(lockPasswordLen)
			if err := ctrl.SetPassword(uc.Username, randomPass); err != nil {
				log.Printf("  %s: FAILED to block: %v", uc.Username, err)
				continue
			}
			log.Printf("  %s: BLOCKED (outside allowed interval)", uc.Username)
			blocked = append(blocked, uc.Username)
			if err := ctrl.DisconnectUserSession(uc.Username); err != nil {
				log.Printf("  %s: session disconnect failed: %v", uc.Username, err)
			}
		}
	}
	log.Printf("Apply access done: unlocked=%v, blocked=%v", unlocked, blocked)
	return nil
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
