//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/aegis/parental-control/internal/adapter/http"
	"github.com/aegis/parental-control/internal/adapter/windows"
	"github.com/aegis/parental-control/internal/usecase/client"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type config struct {
	ServerURL string `yaml:"server_url"`
	ClientID  string `yaml:"client_id"`
}

func main() {
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	installServer := installCmd.String("server-url", "", "Server URL (e.g. http://server:8080)")
	installClientID := installCmd.String("client-id", "", "Client ID (optional, will be generated)")

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
		install(*installServer, *installClientID)
	case "uninstall":
		uninstallCmd.Parse(os.Args[2:])
		uninstall()
	default:
		runService()
	}
}

func runService() {
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

	fetcher := http.NewHTTPConfigFetcher(cfg.ServerURL, cfg.ClientID)
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

func install(serverURL, clientID string) {
	if clientID == "" {
		clientID = uuid.New().String()
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
	fmt.Printf("Компьютер появится в веб-интерфейсе после первого подключения.\n")
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
	exec.Command("sc", "create", "AegisClient", "binPath="+exePath, "start=auto", "DisplayName=Aegis Parental Control Client").Run()
	exec.Command("sc", "start", "AegisClient").Run()
	return nil
}

func stopAndDeleteService() {
	exec.Command("sc", "stop", "AegisClient").Run()
	time.Sleep(2 * time.Second)
	exec.Command("sc", "delete", "AegisClient").Run()
}
