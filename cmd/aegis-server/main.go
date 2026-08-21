package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	httpadapter "github.com/aegis/parental-control/internal/adapter/http"
	"github.com/aegis/parental-control/internal/adapter/jsonfile"
	"github.com/aegis/parental-control/internal/adapter/updates"
)

func main() {
	dataPath := flag.String("data", "aegis-data.json", "Path to JSON data file")
	port := flag.Int("port", 8080, "HTTP port")
	timezone := flag.String("timezone", "Asia/Yekaterinburg", "Timezone for all times (e.g. Asia/Ekaterinburg)")
	updatesDir := flag.String("updates", "", "Directory with client update binaries (default: <data-dir>/updates)")
	flag.Parse()

	loc, err := time.LoadLocation(*timezone)
	if err != nil {
		log.Fatalf("Invalid timezone %q: %v", *timezone, err)
	}

	absPath, err := filepath.Abs(*dataPath)
	if err != nil {
		log.Fatal(err)
	}
	repo, err := jsonfile.New(absPath, loc)
	if err != nil {
		log.Fatal(err)
	}

	updDir := *updatesDir
	if updDir == "" {
		updDir = filepath.Join(filepath.Dir(absPath), "updates")
	}
	updDir, err = filepath.Abs(updDir)
	if err != nil {
		log.Fatal(err)
	}
	_ = os.MkdirAll(updDir, 0755)

	activityStore := jsonfile.NewActivityStore(absPath, loc)
	presenceStore := jsonfile.NewPresenceStore(absPath)
	updateLoader := updates.NewLoader(updDir)

	handler := httpadapter.NewHandler(repo, loc,
		httpadapter.WithActivity(activityStore),
		httpadapter.WithPresence(presenceStore),
		httpadapter.WithUpdates(updateLoader),
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	handler.ServeStatic(mux)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Aegis server starting on http://localhost%s (updates=%s)", addr, updDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
