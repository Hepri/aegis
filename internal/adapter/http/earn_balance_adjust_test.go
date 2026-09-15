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

func TestAdjustEarnBalance(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	repo, err := jsonfile.New(t.TempDir()+"/earn-adj.json", loc)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:   "c1",
		Name: "PC",
		Users: []domain.User{{
			ID: "u1", Name: "Kid", Username: "kid",
			EarnBalanceMinutes: 10,
		}},
		EarnSettings: domain.DefaultEarnSettings(),
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, loc)
	h.earnAdminPassword = "test-pass"
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/clients/c1/users/u1/earn-balance", bytes.NewReader([]byte(`{"delta":15}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Earn-Admin-Password", "test-pass")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("add %d %s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res["balance_minutes"].(float64) != 25 {
		t.Fatalf("after add: %#v", res)
	}

	req = httptest.NewRequest("POST", "/api/clients/c1/users/u1/earn-balance", bytes.NewReader([]byte(`{"delta":-100}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Earn-Admin-Password", "test-pass")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("sub %d %s", rr.Code, rr.Body.String())
	}
	res = map[string]any{}
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res["balance_minutes"].(float64) != 0 {
		t.Fatalf("after sub clamp: %#v", res)
	}

	cs, err := repo.GetClient(context.Background(), "c1")
	if err != nil || cs == nil {
		t.Fatal(err)
	}
	if cs.Users[0].EarnBalanceMinutes != 0 {
		t.Fatalf("persisted balance %d", cs.Users[0].EarnBalanceMinutes)
	}
}
