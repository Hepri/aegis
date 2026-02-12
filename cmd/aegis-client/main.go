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
		log.Fatalf("Open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfgPath := "C:\\Program Files\\Aegis\\aegis-client.yaml"
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfgPath = "aegis-client.yaml"
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalf("Read config: %v", err)
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Parse config: %v", err)
	}
	if cfg.ServerURL == "" || cfg.ClientID == "" {
		log.Fatal("server_url and client_id required in config")
	}

	fetcher := httpadapter.NewHTTPConfigFetcher(cfg.ServerURL, cfg.ClientID)
	ctrl := windows.NewUserControl()

	var lastVersion string

	// Apply on startup
	ctx := context.Background()
	if fetched, err := fetcher.FetchConfig(ctx, ""); err == nil {
		client.ApplyAccess(ctrl, fetched, time.Now())
		lastVersion = fetched.Version
	}

	// Periodic check (every minute) and long-poll
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		fetched, err := fetcher.FetchConfig(ctx, lastVersion)
		cancel()

		if err != nil {
			log.Printf("Fetch config: %v", err)
		} else if fetched != nil {
			client.ApplyAccess(ctrl, fetched, time.Now())
			lastVersion = fetched.Version
		}

		<-ticker.C
	}
}

func install(serverURL, clientID, clientName string) {
	if clientID == "" {
		// Create client on server
		id, err := createClientOnServer(serverURL, clientName)
		if err != nil {
			log.Fatalf("Create client on server: %v", err)
		}
		clientID = id
	}
	installDir := "C:\\Program Files\\Aegis"
	if err := os.MkdirAll(installDir, 0755); err != nil {
		log.Fatalf("Create dir: %v", err)
	}
	cfg := config{ServerURL: serverURL, ClientID: clientID}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatal(err)
	}
	cfgPath := installDir + "\\aegis-client.yaml"
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		log.Fatalf("Write config: %v", err)
	}
	exe, _ := os.Executable()
	dest := installDir + "\\aegis-client.exe"
	if exe != dest {
		if err := copyFile(exe, dest); err != nil {
			log.Fatalf("Copy binary: %v", err)
		}
	}
	if err := createService(dest); err != nil {
		log.Fatalf("Create service: %v", err)
	}
	fmt.Printf("Installed successfully.\n")
	fmt.Printf("Client ID: %s\n", clientID)
	fmt.Printf("Управление: %s\n", serverURL)
}

func createClientOnServer(serverURL, name string) (string, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := http.Post(serverURL+"/api/clients", "application/json", bytes.NewReader(body))
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
	exec.Command("sc", "delete", "AegisClient").Run() // remove if exists
	time.Sleep(500 * time.Millisecond)
	if out, err := exec.Command("sc", "create", "AegisClient", "binPath="+exePath, "start=auto", "DisplayName=Aegis Parental Control Client").CombinedOutput(); err != nil {
		return fmt.Errorf("sc create: %v: %s", err, out)
	}
	if out, err := exec.Command("sc", "start", "AegisClient").CombinedOutput(); err != nil {
		return fmt.Errorf("sc start: %v: %s", err, out)
	}
	return nil
}

func stopAndDeleteService() {
	exec.Command("sc", "stop", "AegisClient").Run()
	time.Sleep(2 * time.Second)
	exec.Command("sc", "delete", "AegisClient").Run()
}
