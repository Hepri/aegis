//go:build windows

package windows

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed earnweb/*
var earnWebFS embed.FS

// RunEarnKiosk starts a local UI (API proxied to Aegis server) and opens Edge fullscreen.
// Network to the server goes only through this process (aegis-client.exe).
func RunEarnKiosk(serverURL, clientID string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("earn kiosk listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	localBase := "http://127.0.0.1:" + strconv.Itoa(port)

	mux := http.NewServeMux()
	sub, err := fs.Sub(earnWebFS, "earnweb")
	if err != nil {
		log.Fatalf("earn web fs: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	target, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		log.Fatalf("server url: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		log.Printf("earn proxy error: %v", e)
		http.Error(w, "сервер Aegis недоступен", http.StatusBadGateway)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("earn kiosk serve: %v", err)
		}
	}()

	openURL := localBase + "/?client_id=" + url.QueryEscape(clientID)
	log.Printf("Earn kiosk UI at %s", openURL)

	edge := edgePath()
	// Must NOT use CREATE_NO_WINDOW / HideWindow — that made the kiosk invisible.
	cmd := exec.Command(edge,
		"--kiosk", openURL,
		"--edge-kiosk-type=fullscreen",
		"--no-first-run",
		"--disable-features=TranslateUI",
		"--check-for-update-interval=31536000",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		log.Printf("start Edge kiosk: %v — trying start via cmd", err)
		fallback := exec.Command("cmd", "/c", "start", "", edge, "--kiosk", openURL, "--edge-kiosk-type=fullscreen")
		fallback.SysProcAttr = &syscall.SysProcAttr{}
		if err2 := fallback.Start(); err2 != nil {
			log.Printf("fallback start failed: %v — server kept at %s", err2, openURL)
			select {}
		}
		_ = fallback.Wait()
		select {}
	}
	_ = cmd.Wait()
	log.Printf("Edge closed, shutting down earn kiosk")
	_ = srv.Close()
	time.Sleep(200 * time.Millisecond)
}
