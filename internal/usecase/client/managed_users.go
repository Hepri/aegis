package client

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/aegis/parental-control/internal/domain"
)

type managedUsersFile struct {
	Usernames []string `json:"usernames"`
}

// ManagedUsersPath is the on-disk list of Windows accounts Aegis manages.
func ManagedUsersPath(dir string) string {
	return filepath.Join(dir, "managed-users.json")
}

// LoadManagedUsers returns usernames from the last successful config fetch.
func LoadManagedUsers(dir string) ([]string, error) {
	data, err := os.ReadFile(ManagedUsersPath(dir))
	if err != nil {
		return nil, err
	}
	var f managedUsersFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Usernames, nil
}

// SaveManagedUsers persists managed Windows usernames for fail-closed boot lock.
func SaveManagedUsers(dir string, usernames []string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(managedUsersFile{Usernames: usernames}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ManagedUsersPath(dir), data, 0644)
}

// UsernamesFromConfig extracts Windows usernames from a client config.
func UsernamesFromConfig(cfg *domain.ClientConfig) []string {
	if cfg == nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Users))
	seen := map[string]struct{}{}
	for _, u := range cfg.Users {
		if u.Username == "" {
			continue
		}
		if _, ok := seen[u.Username]; ok {
			continue
		}
		seen[u.Username] = struct{}{}
		out = append(out, u.Username)
	}
	return out
}
