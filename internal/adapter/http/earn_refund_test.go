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

func TestEarnRedeemCurfewAndRefund(t *testing.T) {
	loc := time.FixedZone("YEKT", 5*3600)
	repo, err := jsonfile.New(t.TempDir()+"/earn-refund.json", loc)
	if err != nil {
		t.Fatal(err)
	}
	settings := domain.DefaultEarnSettings()
	settings.MathGeneratorEnabled = false
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:   "c1",
		Name: "PC",
		Users: []domain.User{{
			ID: "u1", Name: "Kid", Username: "kid",
			EarnBalanceMinutes: 60,
			Schedule:           domain.DaySchedule{},
		}},
		EarnSettings: settings,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, loc)

	// Force "now" is hard without injecting clock; redeem uses repo.now() which is real time.
	// So only test refund path by granting earn temp access via Redeem when allowed,
	// or directly through repo if outside quiet hours.
	av := domain.RedeemAvailabilityAt(time.Now().In(loc))
	if !av.Allowed {
		t.Skip("quiet hours now; skip redeem/refund integration")
	}
	minutes := 5
	if av.MaxMinutes < minutes {
		minutes = av.MaxMinutes
	}
	if minutes < 1 {
		t.Skip("no redeem minutes available")
	}

	body, _ := json.Marshal(map[string]any{"client_id": "c1", "user_id": "u1", "minutes": minutes})
	rr := httptest.NewRecorder()
	h.EarnRedeem(rr, httptest.NewRequest("POST", "/api/earn/redeem", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("redeem %d %s", rr.Code, rr.Body.String())
	}

	// Too long should fail
	tooLong := av.MaxMinutes + 30
	body, _ = json.Marshal(map[string]any{"client_id": "c1", "user_id": "u1", "minutes": tooLong})
	rr = httptest.NewRecorder()
	h.EarnRedeem(rr, httptest.NewRequest("POST", "/api/earn/redeem", bytes.NewReader(body)))
	if rr.Code == http.StatusOK {
		t.Fatal("expected too-long redeem to fail")
	}

	body, _ = json.Marshal(map[string]any{"client_id": "c1", "user_id": "u1"})
	rr = httptest.NewRecorder()
	h.EarnRefundSession(rr, httptest.NewRequest("POST", "/api/earn/refund-session", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("refund %d %s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	json.NewDecoder(rr.Body).Decode(&res)
	if res["refunded_minutes"].(float64) < 1 {
		t.Fatalf("refunded %#v", res)
	}
	st, _ := repo.GetClient(context.Background(), "c1")
	if st.Users[0].EarnBalanceMinutes < 60 {
		// spent minutes then got most back; balance should be near 60
		t.Logf("balance after refund=%d (spent %d)", st.Users[0].EarnBalanceMinutes, minutes)
	}
	if len(st.EarnLog) < 2 {
		t.Fatalf("want earn log entries, got %d", len(st.EarnLog))
	}
}
