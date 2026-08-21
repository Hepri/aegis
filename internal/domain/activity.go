package domain

import "time"

// Event kinds emitted by the Windows client.
const (
	EventSessionLogin  = "session_login"
	EventSessionLogout = "session_logout"
	EventSessionLock   = "session_lock"
	EventSessionUnlock = "session_unlock"
	EventAppOpen       = "app_open"
	EventAppClose      = "app_close"
	EventAppFocus      = "app_focus"
	EventAppBlur       = "app_blur"
)

// ActivityEvent is a single telemetry event from a client.
type ActivityEvent struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	Username   string    `json:"username,omitempty"`
	SessionID  uint32    `json:"session_id,omitempty"`
	AppName    string    `json:"app_name,omitempty"`
	ExePath    string    `json:"exe_path,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
}

// ClientUpdate describes an available client binary update.
type ClientUpdate struct {
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	URL     string `json:"url,omitempty"`
}

// DayActivity is the aggregated activity view for one calendar day.
type DayActivity struct {
	Date     string           `json:"date"`
	Sessions []SessionSummary `json:"sessions"`
}

// SessionSummary is one login–logout (or still-open) session with nested app usage.
type SessionSummary struct {
	SessionID  uint32       `json:"session_id,omitempty"`
	Username   string       `json:"username"`
	Login      time.Time    `json:"login"`
	Logout     *time.Time   `json:"logout,omitempty"`
	DurationMs int64        `json:"duration_ms"`
	LockedMs   int64        `json:"locked_ms,omitempty"`
	LockedNow  bool         `json:"locked_now,omitempty"` // still open, currently on lock screen
	Apps       []AppSummary `json:"apps,omitempty"`
}

// AppSummary aggregates open and focus time for one app.
type AppSummary struct {
	AppName string `json:"app_name"`
	ExePath string `json:"exe_path,omitempty"`
	OpenMs  int64  `json:"open_ms"`
	FocusMs int64  `json:"focus_ms"`
}
