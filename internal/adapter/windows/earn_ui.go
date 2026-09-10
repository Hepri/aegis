//go:build windows

package windows

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

var (
	modUser32         = windows.NewLazySystemDLL("user32.dll")
	procExitWindowsEx = modUser32.NewProc("ExitWindowsEx")
)

const ewxLogoff = 0x00000000

// RunEarnKiosk opens Edge on the server /earn page and serves a local /exit
// endpoint so the web UI can log off the Задачки session.
func RunEarnKiosk(serverURL, clientID string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("earn exit listen: %v", err)
	}
	exitPort := ln.Addr().(*net.TCPAddr).Port
	exitURL := "http://127.0.0.1:" + strconv.Itoa(exitPort) + "/exit"

	mux := http.NewServeMux()
	mux.HandleFunc("/exit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := logoffCurrentSession(); err != nil {
				log.Printf("logoff: %v", err)
			}
		}()
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("aegis-earn-exit"))
	})
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("earn exit server: %v", err)
		}
	}()

	earnURL := strings.TrimRight(serverURL, "/") + "/earn?client_id=" + url.QueryEscape(clientID) +
		"&exit_url=" + url.QueryEscape(exitURL)
	log.Printf("Earn kiosk opening %s (exit=%s)", earnURL, exitURL)

	edge := edgePath()
	for {
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
			log.Printf("start Edge: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		_ = cmd.Wait()
		log.Printf("Edge closed, restarting in 1s")
		time.Sleep(time.Second)
	}
}

func logoffCurrentSession() error {
	r1, _, err := procExitWindowsEx.Call(uintptr(ewxLogoff), 0)
	if r1 == 0 {
		return err
	}
	return nil
}
