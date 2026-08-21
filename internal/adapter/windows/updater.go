//go:build windows

package windows

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// ApplyUpdateIfNeeded downloads a new client binary when the server advertises a newer version.
func ApplyUpdateIfNeeded(localVersion string, update *domain.ClientUpdate, serverURL string) {
	if update == nil || update.Version == "" || update.Version == localVersion {
		return
	}
	log.Printf("OTA: update available %s -> %s", localVersion, update.Version)

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("OTA: executable path: %v", err)
		return
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		log.Printf("OTA: abs path: %v", err)
		return
	}
	dir := filepath.Dir(exePath)
	newPath := filepath.Join(dir, "aegis-client.exe.new")
	oldPath := filepath.Join(dir, "aegis-client.exe.old")

	url := update.URL
	if url == "" {
		url = "/api/updates/aegis-client.exe"
	}
	if strings.HasPrefix(url, "/") {
		url = strings.TrimRight(serverURL, "/") + url
	}

	if err := downloadFile(url, newPath); err != nil {
		log.Printf("OTA: download: %v", err)
		return
	}
	if update.SHA256 != "" {
		sum, err := fileSHA256(newPath)
		if err != nil || !strings.EqualFold(sum, update.SHA256) {
			log.Printf("OTA: sha256 mismatch got=%s want=%s err=%v", sum, update.SHA256, err)
			os.Remove(newPath)
			return
		}
	}

	// On Windows, a running executable can be renamed.
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		log.Printf("OTA: rename current->old: %v", err)
		os.Remove(newPath)
		return
	}
	if err := os.Rename(newPath, exePath); err != nil {
		log.Printf("OTA: rename new->current: %v", err)
		_ = os.Rename(oldPath, exePath) // rollback
		return
	}
	log.Printf("OTA: binary replaced, restarting service")

	cmd := exec.Command("cmd", "/C", "net stop AegisClient & net start AegisClient")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		log.Printf("OTA: restart command: %v", err)
		return
	}
	// Give the restart command a moment, then exit so the service can stop cleanly.
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
