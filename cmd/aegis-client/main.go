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
	"os/exec"
	"path/filepath"
	"time"

	httpadapter "github.com/aegis/parental-control/internal/adapter/http"
	"github.com/aegis/parental-control/internal/adapter/windows"
	"github.com/aegis/parental-control/internal/usecase/client"
	"gopkg.in/yaml.v3"
)

type config struct {
	ServerURL string `yaml:"server_url"`
	ClientID  string `yaml:"client_id"`
}

func main() {
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	installServer := installCmd.String("server-url", "", "Server URL (e.g. http://server:8080)")
	installClientID := installCmd.String("client-id", "", "Client ID (from web UI, or omit with --client-name)")
	installClientName := installCmd.String("client-name", "", "Client name (creates on server if --client-id not set)")

	uninstallCmd := flag.NewFlagSet("uninstall", flag.ExitOnError)

	if len(os.Args) < 2 {
		runService()
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
	default:
		runService()
	}
}

func runService() {
	// Setup logging to file in same directory as exe
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Get executable path: %v", err)
	}
	exeDir := filepath.Dir(exePath)
	logPath := filepath.Join(exeDir, "aegis-client.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// If can't open log file, continue without file logging (will use stderr)
		log.Printf("Warning: cannot open log file %s: %v, logging to stderr", logPath, err)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("=== Aegis Client starting ===")
	log.Printf("Executable path: %s", exePath)
	log.Printf("Log file: %s", logPath)

	cfgPath := "C:\\Program Files\\Aegis\\aegis-client.yaml"
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		log.Printf("Config not found at %s, trying current directory", cfgPath)
		cfgPath = "aegis-client.yaml"
	} else {
		log.Printf("Found config at: %s", cfgPath)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalf("Read config from %s: %v", cfgPath, err)
	}
	log.Printf("Config file read successfully, size: %d bytes", len(data))

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Parse config YAML: %v", err)
	}
	log.Printf("Config parsed: server_url=%s, client_id=%s", cfg.ServerURL, cfg.ClientID)

	if cfg.ServerURL == "" || cfg.ClientID == "" {
		log.Fatal("server_url and client_id required in config")
	}

	log.Printf("Creating config fetcher for server: %s", cfg.ServerURL)
	fetcher := httpadapter.NewHTTPConfigFetcher(cfg.ServerURL, cfg.ClientID)
	log.Printf("Creating user control")
	ctrl := windows.NewUserControl()

	var lastVersion string

	// Apply on startup
	log.Printf("Fetching initial config from server...")
	ctx := context.Background()
	fetched, err := fetcher.FetchConfig(ctx, "")
	if err != nil {
		log.Printf("Failed to fetch initial config: %v", err)
	} else if fetched != nil {
		log.Printf("Initial config received, version: %s, users: %d", fetched.Version, len(fetched.Users))
		log.Printf("Applying access rules...")
		client.ApplyAccess(ctrl, fetched, time.Now())
		lastVersion = fetched.Version
		log.Printf("Access rules applied successfully")
	} else {
		log.Printf("No config received (server may not have this client registered)")
	}

	// Periodic check (every minute) and long-poll
	log.Printf("Starting periodic config check (every 1 minute)")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	iteration := 0
	for {
		iteration++
		log.Printf("=== Config check iteration %d ===", iteration)
		log.Printf("Fetching config (last version: %s)...", lastVersion)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		fetched, err := fetcher.FetchConfig(ctx, lastVersion)
		cancel()

		if err != nil {
			log.Printf("Fetch config error: %v", err)
		} else if fetched != nil {
			if fetched.Version != lastVersion {
				log.Printf("Config updated: version %s -> %s, users: %d", lastVersion, fetched.Version, len(fetched.Users))
				log.Printf("Applying new access rules...")
				client.ApplyAccess(ctrl, fetched, time.Now())
				lastVersion = fetched.Version
				log.Printf("Access rules updated successfully")
			} else {
				log.Printf("Config unchanged (version: %s)", lastVersion)
			}
		} else {
			log.Printf("No config received")
		}

		<-ticker.C
	}
}

func install(serverURL, clientID, clientName string) {
	fmt.Printf("=== Aegis Client Installation ===\n")
	fmt.Printf("Server URL: %s\n", serverURL)

	if clientID == "" {
		fmt.Printf("Creating client on server with name: %s\n", clientName)
		log.Printf("Creating client on server: %s", serverURL)
		id, err := createClientOnServer(serverURL, clientName)
		if err != nil {
			log.Fatalf("Create client on server: %v", err)
		}
		clientID = id
		fmt.Printf("Client created on server, ID: %s\n", clientID)
		log.Printf("Client ID received: %s", clientID)
	} else {
		fmt.Printf("Using existing client ID: %s\n", clientID)
		log.Printf("Using provided client ID: %s", clientID)
	}

	installDir := "C:\\Program Files\\Aegis"
	fmt.Printf("Creating installation directory: %s\n", installDir)
	log.Printf("Creating directory: %s", installDir)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		log.Fatalf("Create dir: %v", err)
	}
	fmt.Printf("Directory created\n")

	cfg := config{ServerURL: serverURL, ClientID: clientID}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatal(err)
	}
	cfgPath := installDir + "\\aegis-client.yaml"
	fmt.Printf("Writing config file: %s\n", cfgPath)
	log.Printf("Writing config to: %s", cfgPath)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		log.Fatalf("Write config: %v", err)
	}
	fmt.Printf("Config file written\n")

	exe, _ := os.Executable()
	dest := installDir + "\\aegis-client.exe"
	fmt.Printf("Copying executable: %s -> %s\n", exe, dest)
	log.Printf("Copying executable from %s to %s", exe, dest)
	if exe != dest {
		if err := copyFile(exe, dest); err != nil {
			log.Fatalf("Copy binary: %v", err)
		}
		fmt.Printf("Executable copied\n")
	} else {
		fmt.Printf("Executable already in target location\n")
	}

	fmt.Printf("Creating Windows service...\n")
	log.Printf("Creating service with path: %s", dest)
	if err := createService(dest); err != nil {
		log.Fatalf("Create service: %v", err)
	}
	fmt.Printf("Service created and started\n")

	fmt.Printf("\n=== Installation Complete ===\n")
	fmt.Printf("Client ID: %s\n", clientID)
	fmt.Printf("Управление: %s\n", serverURL)
	fmt.Printf("Log file: %s\\aegis-client.log\n", installDir)
}

func createClientOnServer(serverURL, name string) (string, error) {
	url := serverURL + "/api/clients"
	log.Printf("POST %s with name: %s", url, name)
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("HTTP POST error: %v", err)
		return "", err
	}
	defer resp.Body.Close()
	log.Printf("Server response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Printf("JSON decode error: %v", err)
		return "", err
	}
	if r.ID == "" {
		return "", fmt.Errorf("server returned empty client id")
	}
	log.Printf("Client created successfully, ID: %s", r.ID)
	return r.ID, nil
}

func uninstall() {
	stopAndDeleteService()
	os.RemoveAll("C:\\Program Files\\Aegis")
	fmt.Println("Uninstalled.")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func createService(exePath string) error {
	log.Printf("Deleting existing service (if any)...")
	exec.Command("sc", "delete", "AegisClient").Run() // remove if exists
	time.Sleep(500 * time.Millisecond)

	log.Printf("Creating service: AegisClient, path: %s", exePath)
	if out, err := exec.Command("sc", "create", "AegisClient", "binPath="+exePath, "start=auto", "DisplayName=Aegis Parental Control Client").CombinedOutput(); err != nil {
		log.Printf("sc create failed: %v, output: %s", err, out)
		return fmt.Errorf("sc create: %v: %s", err, out)
	}
	log.Printf("Service created successfully")
	time.Sleep(1 * time.Second) // Wait for service to be registered

	log.Printf("Starting service...")
	if out, err := exec.Command("sc", "start", "AegisClient").CombinedOutput(); err != nil {
		log.Printf("sc start failed: %v, output: %s", err, out)
		// Check service status for more info
		statusOut, _ := exec.Command("sc", "query", "AegisClient").CombinedOutput()
		log.Printf("Service status: %s", statusOut)
		return fmt.Errorf("sc start failed: %v\nOutput: %s\nService status: %s", err, out, statusOut)
	}
	log.Printf("Service start command executed")
	time.Sleep(2 * time.Second) // Wait for service to start

	log.Printf("Checking service status...")
	// Check if service is actually running
	if out, err := exec.Command("sc", "query", "AegisClient").CombinedOutput(); err != nil {
		log.Printf("Failed to query service: %v, output: %s", err, out)
		return fmt.Errorf("failed to query service status: %v: %s", err, out)
	}
	log.Printf("Service status check completed")
	return nil
}

func stopAndDeleteService() {
	exec.Command("sc", "stop", "AegisClient").Run()
	time.Sleep(2 * time.Second)
	exec.Command("sc", "delete", "AegisClient").Run()
}
