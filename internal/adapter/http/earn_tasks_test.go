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

func TestCustomEarnTaskCRUD(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	repo, err := jsonfile.New(t.TempDir()+"/earn-tasks.json", loc)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:   "c1",
		Name: "PC",
		Users: []domain.User{{
			ID: "u1", Name: "Kid", Username: "kid",
		}},
		EarnSettings: domain.DefaultEarnSettings(),
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, loc)
	h.earnAdminPassword = "test-pass"

	auth := func(r *http.Request) {
		r.Header.Set("X-Earn-Admin-Password", "test-pass")
	}

	body, _ := json.Marshal(map[string]any{
		"subject":        "мораль",
		"prompt":         "Можно ли обманывать?",
		"answer":         "нет",
		"choices":        []string{"да", "нет"},
		"reward_minutes": 2,
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/clients/c1/earn-tasks", bytes.NewReader(body))
	req.SetPathValue("id", "c1")
	auth(req)
	h.UpsertEarnTask(rr, req)
	if rr.Code != 200 {
		t.Fatalf("create status %d body %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	id, _ := created["id"].(string)
	if id == "" || created["editable"] != true {
		t.Fatalf("created=%#v", created)
	}

	st, _ := repo.GetClient(context.Background(), "c1")
	if len(st.EarnTasks) != 1 || st.EarnTasks[0].Prompt != "Можно ли обманывать?" {
		t.Fatalf("stored=%+v", st.EarnTasks)
	}

	// Builtin cannot be overwritten
	builtin := domain.BuiltInEarnBank()[0]
	body, _ = json.Marshal(map[string]any{
		"id":      builtin.ID,
		"subject": "мораль",
		"prompt":  "x",
		"answer":  "y",
	})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/clients/c1/earn-tasks", bytes.NewReader(body))
	req.SetPathValue("id", "c1")
	auth(req)
	h.UpsertEarnTask(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("builtin overwrite status %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/api/clients/c1/earn-tasks/"+id, nil)
	req.SetPathValue("id", "c1")
	req.SetPathValue("taskId", id)
	auth(req)
	h.DeleteEarnTask(rr, req)
	if rr.Code != 200 {
		t.Fatalf("delete status %d %s", rr.Code, rr.Body.String())
	}
	st, _ = repo.GetClient(context.Background(), "c1")
	if len(st.EarnTasks) != 0 {
		t.Fatalf("want empty after delete, got %+v", st.EarnTasks)
	}
}
