//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	httpadapter "github.com/aegis/parental-control/internal/adapter/http"
	"github.com/aegis/parental-control/internal/adapter/windows"
	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/usecase/client"
	"github.com/kardianos/service"
	"gopkg.in/yaml.v3"
)

// Version is set via -ldflags "-X main.Version=..."
var Version = "dev"

type config struct {
	ServerURL string `yaml:"server_url"`
	ClientID  string `yaml:"client_id"`
}

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	p.exit = make(chan struct{})
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	return nil
}

func (p *program) run() {
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Get executable path: %v", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	logPath := filepath.Join(exeDir, "aegis-client.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: cannot open log file %s: %v", logPath, err)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("=== Aegis Client starting === version=%s", Version)
	log.Printf("Executable path: %s", exePath)

	cfgPath := `C:\Program Files\Aegis\aegis-client.yaml`
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfgPath = filepath.Join(exeDir, "aegis-client.yaml")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Printf("Read config from %s: %v", cfgPath, err)
		return
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("Parse config YAML: %v", err)
		return
	}
	if cfg.ServerURL == "" || cfg.ClientID == "" {
		log.Printf("server_url and client_id required in config")
		return
	}

	fetcher := httpadapter.NewHTTPConfigFetcher(cfg.ServerURL, cfg.ClientID)
	ctrl := windows.NewUserControl()
	uploader := httpadapter.NewEventUploader(cfg.ServerURL, cfg.ClientID, filepath.Join(exeDir, "events-queue.jsonl"))

	agentMgr := windows.NewSessionAgentManager(exePath)
	_ = agentMgr.Start()
	defer agentMgr.Stop()

	var currentConfig *domain.ClientConfig
	var lastVersion string
	var lastState map[string]bool
	prevSessions := map[uint32]client.SessionSnapshot{}
	prevApps := map[uint32]*client.AppWatchState{}
	openSince := map[uint32]map[string]time.Time{}
	focusSince := map[uint32]*time.Time{}
	focusKey := map[uint32]string{}

	go func() {
		for {
			select {
			case <-p.exit:
				return
			default:
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			fetched, err := fetcher.FetchConfig(ctx, lastVersion)
			cancel()
			if err != nil {
				log.Printf("Fetch config error: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if fetched == nil {
				continue
			}
			if fetched.Version != lastVersion {
				log.Printf("Config updated: version %s -> %s", lastVersion, fetched.Version)
				currentConfig = fetched
				lastVersion = fetched.Version
			}
			if fetched.Update != nil {
				windows.ApplyUpdateIfNeeded(Version, fetched.Update, cfg.ServerURL)
			}
		}
	}()

	log.Printf("Fetching initial config from server...")
	ctx := context.Background()
	fetched, err := fetcher.FetchConfig(ctx, "")
	if err != nil {
		log.Printf("Failed to fetch initial config: %v", err)
	} else if fetched != nil {
		log.Printf("Initial config received, version: %s, users: %d", fetched.Version, len(fetched.Users))
		currentConfig = fetched
		lastVersion = fetched.Version
		lastState = client.ApplyAccessIfNeeded(ctrl, fetched, time.Now(), nil)
		if fetched.Update != nil {
			windows.ApplyUpdateIfNeeded(Version, fetched.Update, cfg.ServerURL)
		}
	}

	stateTicker := time.NewTicker(10 * time.Second)
	defer stateTicker.Stop()
	activityTicker := time.NewTicker(5 * time.Second)
	defer activityTicker.Stop()
	uploadTicker := time.NewTicker(15 * time.Second)
	defer uploadTicker.Stop()

	for {
		select {
		case <-p.exit:
			log.Printf("Service stopping")
			return
		case <-stateTicker.C:
			if currentConfig != nil {
				lastState = client.ApplyAccessIfNeeded(ctrl, currentConfig, time.Now(), lastState)
			}
		case <-activityTicker.C:
			now := time.Now()
			sessions, err := windows.ListSessions()
			if err != nil {
				log.Printf("ListSessions: %v", err)
				continue
			}
			ev := client.DiffSessions(prevSessions, sessions, now)
			if len(ev) > 0 {
				if err := uploader.Enqueue(ev); err != nil {
					log.Printf("enqueue session events: %v", err)
				}
			}
			agentMgr.SyncAgents(sessions)

			appStates := agentMgr.LatestStates()
			for sid, state := range appStates {
				prev := prevApps[sid]
				osMap := openSince[sid]
				if osMap == nil {
					osMap = map[string]time.Time{}
				}
				appEv, newOpen, newFS, newFK := client.DiffApps(prev, &state, now, osMap, focusSince[sid], focusKey[sid])
				if len(appEv) > 0 {
					if err := uploader.Enqueue(appEv); err != nil {
						log.Printf("enqueue app events: %v", err)
					}
				}
				cp := state
				prevApps[sid] = &cp
				openSince[sid] = newOpen
				focusSince[sid] = newFS
				focusKey[sid] = newFK
			}
			for sid := range prevApps {
				if _, ok := sessions[sid]; !ok {
					delete(prevApps, sid)
					delete(openSince, sid)
					delete(focusSince, sid)
					delete(focusKey, sid)
				}
			}
			prevSessions = sessions
		case <-uploadTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			if err := uploader.Flush(ctx); err != nil {
				log.Printf("upload events: %v", err)
			}
			cancel()
		}
	}
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "session-agent" {
		fs := flag.NewFlagSet("session-agent", flag.ExitOnError)
		sid := fs.Uint("session-id", 0, "WTS session id")
		user := fs.String("username", "", "Windows username")
		_ = fs.Parse(os.Args[2:])
		windows.RunSessionAgent(uint32(*sid), *user)
		return
	}

	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	installServer := installCmd.String("server-url", "", "Server URL (e.g. http://server:8080)")
	installClientID := installCmd.String("client-id", "", "Client ID (from web UI, or omit with --client-name)")
	installClientName := installCmd.String("client-name", "", "Client name (creates on server if --client-id not set)")

	uninstallCmd := flag.NewFlagSet("uninstall", flag.ExitOnError)

	if len(os.Args) < 2 {
		runAsService()
		return
	}

	switch os.Args[1] {
	case "install":
		installCmd.Parse(os.Args[2:])
		if *installServer == "" {
			log.Fatal("--server-url required")
		}
		if *installClientID == "" && *installClientName == "" {
			log.Fatal("--client-id or --client-name required")
		}
		install(*installServer, *installClientID, *installClientName)
	case "uninstall":
		uninstallCmd.Parse(os.Args[2:])
		uninstall()
	case "version":
		fmt.Println(Version)
	default:
		runAsService()
	}
}

func runAsService() {
	prg := &program{}
	svcConfig := &service.Config{
		Name:        "AegisClient",
		DisplayName: "Aegis Parental Control Client",
		Description: "Parental control client that enforces access schedules",
		Executable:  `C:\Program Files\Aegis\aegis-client.exe`,
	}

	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func install(serverURL, clientID, clientName string) {
	fmt.Printf("=== Aegis Client Installation ===\n")
	fmt.Printf("Server URL: %s\n", serverURL)
	fmt.Printf("Version: %s\n", Version)

	if clientID == "" {
		fmt.Printf("Creating client on server with name: %s\n", clientName)
		id, err := createClientOnServer(serverURL, clientName)
		if err != nil {
			log.Fatalf("Create client on server: %v", err)
		}
		clientID = id
		fmt.Printf("Client created on server, ID: %s\n", clientID)
	} else {
		fmt.Printf("Using existing client ID: %s\n", clientID)
	}

	installDir := `C:\Program Files\Aegis`
	fmt.Printf("Creating installation directory: %s\n", installDir)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		log.Fatalf("Create dir: %v", err)
	}

	cfg := config{ServerURL: serverURL, ClientID: clientID}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatal(err)
	}
	cfgPath := installDir + `\aegis-client.yaml`
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		log.Fatalf("Write config: %v", err)
	}

	exe, _ := os.Executable()
	dest := installDir + `\aegis-client.exe`
	fmt.Printf("Copying executable: %s -> %s\n", exe, dest)
	if exe != dest {
		if err := copyFile(exe, dest); err != nil {
			log.Fatalf("Copy binary: %v", err)
		}
	}

	prg := &program{}
	svcConfig := &service.Config{
		Name:        "AegisClient",
		DisplayName: "Aegis Parental Control Client",
		Description: "Parental control client that enforces access schedules",
		Executable:  dest,
	}

	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("Create service: %v", err)
	}

	if err := s.Install(); err != nil {
		log.Fatalf("Install service: %v", err)
	}
	if err := s.Start(); err != nil {
		log.Fatalf("Start service: %v", err)
	}

	fmt.Printf("\n=== Installation Complete ===\n")
	fmt.Printf("Client ID: %s\n", clientID)
	fmt.Printf("Управление: %s\n", serverURL)
	fmt.Printf("Log file: %s\\aegis-client.log\n", installDir)
}

func createClientOnServer(serverURL, name string) (string, error) {
	url := serverURL + "/api/clients"
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.ID == "" {
		return "", fmt.Errorf("server returned empty client id")
	}
	return r.ID, nil
}

func uninstall() {
	prg := &program{}
	svcConfig := &service.Config{
		Name:       "AegisClient",
		Executable: `C:\Program Files\Aegis\aegis-client.exe`,
	}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("Create service: %v", err)
	}
	s.Stop()
	time.Sleep(2 * time.Second)
	if err := s.Uninstall(); err != nil {
		log.Printf("Uninstall: %v", err)
	}
	os.RemoveAll(`C:\Program Files\Aegis`)
	fmt.Println("Uninstalled.")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}
