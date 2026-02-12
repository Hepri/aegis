package domain

import "time"

// AllowedInterval represents a time window when user has access
type AllowedInterval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// UserAccessConfig is per-user config sent to client
type UserAccessConfig struct {
	Username         string            `json:"username"`
	AllowedIntervals []AllowedInterval `json:"allowed_intervals"`
}

// ClientConfig is the full config sent to client (declarative)
type ClientConfig struct {
	Users   []UserAccessConfig `json:"users"`
	Version string             `json:"version"`
}
