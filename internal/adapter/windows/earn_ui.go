//go:build windows

package windows

import (
	"io"
	"log"
	"net/url"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// RunEarnKiosk opens Edge fullscreen on the Aegis server /earn page (no localhost proxy).
func RunEarnKiosk(serverURL, clientID string) {
	earnURL := strings.TrimRight(serverURL, "/") + "/earn?client_id=" + url.QueryEscape(clientID)
	log.Printf("Earn kiosk opening %s", earnURL)

	edge := edgePath()
	cmd := exec.Command(edge,
		"--kiosk", earnURL,
		"--edge-kiosk-type=fullscreen",
		"--no-first-run",
		"--disable-features=TranslateUI",
		"--check-for-update-interval=31536000",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		log.Printf("start Edge: %v, trying cmd start", err)
		fallback := exec.Command("cmd", "/c", "start", "", edge,
			"--kiosk", earnURL,
			"--edge-kiosk-type=fullscreen",
			"--no-first-run",
		)
		fallback.SysProcAttr = &syscall.SysProcAttr{}
		if err2 := fallback.Run(); err2 != nil {
			log.Fatalf("open Edge failed: %v / %v", err, err2)
		}
		time.Sleep(24 * time.Hour)
		return
	}
	_ = cmd.Wait()
	log.Printf("Edge closed")
}
