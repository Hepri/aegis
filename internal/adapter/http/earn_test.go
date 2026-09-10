package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegis/parental-control/internal/adapter/jsonfile"
	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
)

func TestEarnEmptyTasksAndRedeem(t *testing.T) {
	repo, err := jsonfile.New(t.TempDir()+"/earn.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{
		ID:                 "u1",
		Name:               "Kid",
		Username:           "kid",
		Schedule:           domain.DaySchedule{},
		EarnBalanceMinutes: 20,
	}
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:           "c1",
		Name:         "PC",
		Users:        []domain.User{user},
		EarnTasks:    []domain.EarnTask{},
		EarnSettings: domain.DefaultEarnSettings(),
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, nil)

	req := httptest.NewRequest("GET", "/api/earn/next?client_id=c1&user_id=u1", nil)
	rr := httptest.NewRecorder()
	h.EarnNext(rr, req)
	if rr.Code != 200 {
		t.Fatalf("next status %d", rr.Code)
	}
	var next map[string]any
	json.NewDecoder(rr.Body).Decode(&next)
	if next["empty"] != true {
		t.Fatalf("expected empty next, got %#v", next)
	}

	body, _ := json.Marshal(map[string]any{"client_id": "c1", "user_id": "u1", "minutes": 15})
	req = httptest.NewRequest("POST", "/api/earn/redeem", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	h.EarnRedeem(rr, req)
	if rr.Code != 200 {
		t.Fatalf("redeem status %d body %s", rr.Code, rr.Body.String())
	}

	st, _ := repo.GetClient(context.Background(), "c1")
	if st.Users[0].EarnBalanceMinutes != 5 {
		t.Fatalf("balance=%d want 5", st.Users[0].EarnBalanceMinutes)
	}
	if len(st.TemporaryAccessRequests) != 1 {
		t.Fatalf("temp access count=%d", len(st.TemporaryAccessRequests))
	}
}

func TestEarnAnswerCreditsBalance(t *testing.T) {
	repo, err := jsonfile.New(t.TempDir()+"/earn2.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:   "c1",
		Name: "PC",
		Users: []domain.User{{
			ID: "u1", Name: "Kid", Username: "kid", Schedule: domain.DaySchedule{},
		}},
		EarnTasks: []domain.EarnTask{{
			ID: "t1", Prompt: "2+2?", Answer: "4", RewardMinutes: 10, Enabled: true,
		}},
		EarnSettings: domain.DefaultEarnSettings(),
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, nil)

	body, _ := json.Marshal(map[string]any{
		"client_id": "c1", "user_id": "u1", "task_id": "t1", "answer": "4",
	})
	req := httptest.NewRequest("POST", "/api/earn/answer", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.EarnAnswer(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	json.NewDecoder(rr.Body).Decode(&res)
	if res["correct"] != true {
		t.Fatalf("want correct: %#v", res)
	}
	if int(res["balance_minutes"].(float64)) != 10 {
		t.Fatalf("balance %#v", res["balance_minutes"])
	}

	// second solve blocked
	req = httptest.NewRequest("POST", "/api/earn/answer", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	h.EarnAnswer(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("second solve status %d", rr.Code)
	}
}
