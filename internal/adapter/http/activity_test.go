package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/adapter/jsonfile"
	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

func TestPostEventsAndGetActivity(t *testing.T) {
	dir := t.TempDir()
	repo, err := jsonfile.New(dir+"/data.json", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	activity := jsonfile.NewActivityStore(dir+"/data.json", time.UTC)
	presence := jsonfile.NewPresenceStore(dir + "/data.json")
	h := NewHandler(repo, time.UTC, WithActivity(activity), WithPresence(presence))

	clientID := "client-act-1"
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID: clientID, Name: "PC",
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{
		"events": []domain.ActivityEvent{
			{Type: domain.EventSessionLogin, Timestamp: now, Username: "kid", SessionID: 2},
			{Type: domain.EventAppOpen, Timestamp: now, Username: "kid", AppName: "Notepad", ExePath: "notepad.exe"},
			{Type: domain.EventAppFocus, Timestamp: now, Username: "kid", AppName: "Notepad", ExePath: "notepad.exe"},
		},
	})
	req := httptest.NewRequest("POST", "/api/clients/"+clientID+"/events", bytes.NewReader(body))
	req.SetPathValue("id", clientID)
	rr := httptest.NewRecorder()
	h.PostEvents(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("post events status=%d body=%s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest("GET", "/api/clients/"+clientID+"/activity?date=2026-08-21", nil)
	req2.SetPathValue("id", clientID)
	rr2 := httptest.NewRecorder()
	h.GetActivity(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("get activity status=%d", rr2.Code)
	}
	var agg domain.DayActivity
	if err := json.NewDecoder(rr2.Body).Decode(&agg); err != nil {
		t.Fatal(err)
	}
	if len(agg.Sessions) != 1 {
		t.Fatalf("sessions=%d", len(agg.Sessions))
	}
	if len(agg.Apps) != 1 || agg.Apps[0].AppName != "Notepad" {
		t.Fatalf("apps=%+v", agg.Apps)
	}
}
