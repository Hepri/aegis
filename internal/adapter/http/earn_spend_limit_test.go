package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/adapter/jsonfile"
	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

func TestRedeemDailySpendLimitRussian(t *testing.T) {
	loc := time.Local
	repo, err := jsonfile.New(t.TempDir()+"/earn-spend.json", loc)
	if err != nil {
		t.Fatal(err)
	}
	settings := domain.DefaultEarnSettings()
	settings.MaxEarnPerDay = 30
	// Cover all weekdays with default-like full day so curfew schedule is not the limiter.
	longDay := []domain.TimeInterval{{Start: "00:00", End: "00:00"}}
	settings.RedeemSchedule = domain.DaySchedule{}
	for _, d := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		settings.RedeemSchedule[d] = append([]domain.TimeInterval(nil), longDay...)
	}
	firstBuy, overBuy := 20, 15
	av := domain.RedeemAvailabilityAt(time.Now().In(loc), settings.RedeemSchedule)
	if !av.Allowed || av.MaxMinutes < 1 {
		t.Skipf("redeem blocked: %+v", av)
	}
	if av.MaxMinutes < 25 {
		// Near midnight the window is short — still exercise the spend-cap path.
		settings.MaxEarnPerDay = 2
		firstBuy, overBuy = 1, 2
	}
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:   "c1",
		Name: "PC",
		Users: []domain.User{{
			ID: "u1", Name: "Kid", Username: "kid",
			EarnBalanceMinutes: 200,
		}},
		EarnSettings: settings,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, loc)

	buy := func(minutes int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"client_id": "c1", "user_id": "u1", "minutes": minutes})
		rr := httptest.NewRecorder()
		h.EarnRedeem(rr, httptest.NewRequest("POST", "/api/earn/redeem", bytes.NewReader(body)))
		return rr
	}

	rr := buy(firstBuy)
	if rr.Code != 200 {
		t.Fatalf("first redeem status %d body %s", rr.Code, rr.Body.String())
	}
	rr = buy(overBuy)
	if rr.Code != 409 {
		t.Fatalf("over-limit status %d want 409 body %s", rr.Code, rr.Body.String())
	}
	var errBody map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&errBody); err != nil {
		t.Fatal(err)
	}
	msg, _ := errBody["message"].(string)
	wantSpent := fmt.Sprintf("уже потрачено %d из %d", firstBuy, settings.MaxEarnPerDay)
	if !strings.Contains(msg, wantSpent) {
		t.Fatalf("message=%q want %q", msg, wantSpent)
	}
	if !errors.Is(domain.DailySpendLimitError(firstBuy, settings.MaxEarnPerDay, overBuy), domain.ErrDailyLimit) {
		t.Fatal("DailySpendLimitError should wrap ErrDailyLimit")
	}

	st, _ := repo.GetClient(context.Background(), "c1")
	if st.Users[0].EarnDayStats.SpentMinutes != firstBuy {
		t.Fatalf("spent=%d want %d", st.Users[0].EarnDayStats.SpentMinutes, firstBuy)
	}
	if st.Users[0].EarnBalanceMinutes != 200-firstBuy {
		t.Fatalf("balance=%d want %d", st.Users[0].EarnBalanceMinutes, 200-firstBuy)
	}
}
