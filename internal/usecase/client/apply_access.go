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
	for _, uc := range config.Users {
		allowed := isWithinIntervals(now, uc.AllowedIntervals)
		if allowed {
			if err := ctrl.SetPassword(uc.Username, unlockPassword); err != nil {
				log.Printf("Failed to activate user %s: %v", uc.Username, err)
				continue
			}
			log.Printf("User %s: activated (access allowed)", uc.Username)
		} else {
			randomPass := generateRandomPassword(lockPasswordLen)
			if err := ctrl.SetPassword(uc.Username, randomPass); err != nil {
				log.Printf("Failed to deactivate user %s: %v", uc.Username, err)
				continue
			}
			log.Printf("User %s: deactivated (access denied)", uc.Username)
			if err := ctrl.DisconnectUserSession(uc.Username); err != nil {
				log.Printf("User %s: session disconnect failed: %v", uc.Username, err)
			}
		}
	}
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
