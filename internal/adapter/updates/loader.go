package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// Manifest is the on-disk client update descriptor (client.json).
type Manifest struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	File    string `json:"file"` // relative filename, e.g. aegis-client.exe
}

// Loader reads update manifests from a directory without requiring a server restart.
type Loader struct {
	mu      sync.Mutex
	dir     string
	baseURL string // e.g. "" meaning relative /api/updates/...
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) Dir() string {
	return l.dir
}

// GetClientUpdate returns the current update info, or nil if none available.
func (l *Loader) GetClientUpdate() *domain.ClientUpdate {
	if l == nil || l.dir == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	manifestPath := filepath.Join(l.dir, "client.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil || m.Version == "" {
		return nil
	}
	fileName := m.File
	if fileName == "" {
		fileName = "aegis-client.exe"
	}
	binPath := filepath.Join(l.dir, fileName)
	if _, err := os.Stat(binPath); err != nil {
		return nil
	}
	sha := m.SHA256
	if sha == "" {
		sha, _ = fileSHA256(binPath)
	}
	return &domain.ClientUpdate{
		Version: m.Version,
		SHA256:  sha,
		URL:     "/api/updates/aegis-client.exe",
	}
}

// BinaryPath returns the path to the client binary if present.
func (l *Loader) BinaryPath() (string, error) {
	if l == nil || l.dir == "" {
		return "", os.ErrNotExist
	}
	manifestPath := filepath.Join(l.dir, "client.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// Fall back to default name
		p := filepath.Join(l.dir, "aegis-client.exe")
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	fileName := m.File
	if fileName == "" {
		fileName = "aegis-client.exe"
	}
	return filepath.Join(l.dir, fileName), nil
}

// WriteManifest writes client.json after placing the binary.
func WriteManifest(dir, version, fileName string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	binPath := filepath.Join(dir, fileName)
	sha, err := fileSHA256(binPath)
	if err != nil {
		return err
	}
	m := Manifest{Version: version, SHA256: sha, File: fileName}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "client.json"), data, 0644)
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ModTime of client.json for cache-busting diagnostics.
func (l *Loader) ModTime() time.Time {
	if l == nil {
		return time.Time{}
	}
	fi, err := os.Stat(filepath.Join(l.dir, "client.json"))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
