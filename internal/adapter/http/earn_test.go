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

func TestEarnNextGeneratesMathWhenBankEmpty(t *testing.T) {
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
	if next["empty"] == true {
		t.Fatalf("expected generated task, got empty: %#v", next)
	}
	if next["prompt"] == nil || next["id"] == nil {
		t.Fatalf("expected prompt/id: %#v", next)
	}

	// Sticky: second next returns same id
	id1 := next["id"].(string)
	rr = httptest.NewRecorder()
	h.EarnNext(rr, httptest.NewRequest("GET", "/api/earn/next?client_id=c1&user_id=u1", nil))
	var next2 map[string]any
	json.NewDecoder(rr.Body).Decode(&next2)
	if next2["id"] != id1 {
		t.Fatalf("sticky want %s got %#v", id1, next2["id"])
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

	rr := httptest.NewRecorder()
	h.EarnNext(rr, httptest.NewRequest("GET", "/api/earn/next?client_id=c1&user_id=u1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("next %d %s", rr.Code, rr.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"client_id": "c1", "user_id": "u1", "task_id": "t1", "answer": "4",
	})
	req := httptest.NewRequest("POST", "/api/earn/answer", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	h.EarnAnswer(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var res domain.EarnAnswerResult
	json.NewDecoder(rr.Body).Decode(&res)
	if !res.Correct {
		t.Fatalf("want correct: %#v", res)
	}
	if res.Balance != 10 {
		t.Fatalf("balance %#v", res.Balance)
	}

	// second solve blocked (no active challenge / already solved)
	req = httptest.NewRequest("POST", "/api/earn/answer", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	h.EarnAnswer(rr, req)
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusConflict {
		t.Fatalf("second solve status %d", rr.Code)
	}
}

func TestEarnWrongStreakPenaltyAndReplace(t *testing.T) {
	repo, err := jsonfile.New(t.TempDir()+"/earn3.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	settings := domain.DefaultEarnSettings()
	settings.WrongLockSeconds = 1
	settings.WrongStreakLimit = 2
	settings.WrongStreakPenaltyMinutes = 1
	settings.MathGeneratorEnabled = false
	if err := repo.SaveClient(context.Background(), &port.ClientState{
		ID:   "c1",
		Name: "PC",
		Users: []domain.User{{
			ID: "u1", Name: "Kid", Username: "kid", Schedule: domain.DaySchedule{},
			EarnBalanceMinutes: 5,
		}},
		EarnTasks: []domain.EarnTask{
			{ID: "t1", Prompt: "1+1?", Answer: "2", RewardMinutes: 1, Enabled: true},
			{ID: "t2", Prompt: "2+2?", Answer: "4", RewardMinutes: 1, Enabled: true},
		},
		EarnSettings: settings,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(repo, nil)

	rr := httptest.NewRecorder()
	h.EarnNext(rr, httptest.NewRequest("GET", "/api/earn/next?client_id=c1&user_id=u1", nil))
	var next map[string]any
	json.NewDecoder(rr.Body).Decode(&next)
	taskID := next["id"].(string)

	answerWrong := func() domain.EarnAnswerResult {
		// Clear lock by answering after unlocking via direct store mutation is hard;
		// use WrongLockSeconds=1 and sleep, or zero lock for test via settings update.
		body, _ := json.Marshal(map[string]any{
			"client_id": "c1", "user_id": "u1", "task_id": taskID, "answer": "0",
		})
		r := httptest.NewRecorder()
		h.EarnAnswer(r, httptest.NewRequest("POST", "/api/earn/answer", bytes.NewReader(body)))
		if r.Code == http.StatusLocked {
			t.Fatal("locked unexpectedly")
		}
		if r.Code != http.StatusOK {
			t.Fatalf("answer status %d %s", r.Code, r.Body.String())
		}
		var res domain.EarnAnswerResult
		json.NewDecoder(r.Body).Decode(&res)
		return res
	}

	// Disable lock for streak testing via settings
	settings.WrongLockSeconds = 1
	_ = repo.UpdateEarnSettings(context.Background(), "c1", settings)

	res1 := answerWrong()
	if res1.Correct || res1.ReplaceQuestion {
		t.Fatalf("first wrong: %#v", res1)
	}

	// Clear lock on user for second attempt
	st, _ := repo.GetClient(context.Background(), "c1")
	st.Users[0].EarnLockedUntil = 0
	_ = repo.SaveClient(context.Background(), st)

	res2 := answerWrong()
	if !res2.ReplaceQuestion {
		t.Fatalf("want replace on streak: %#v", res2)
	}
	if res2.PenaltyMinutes != 1 {
		t.Fatalf("penalty %#v", res2.PenaltyMinutes)
	}
	if res2.Balance != 4 {
		t.Fatalf("balance after penalty %#v", res2.Balance)
	}

	rr = httptest.NewRecorder()
	h.EarnNext(rr, httptest.NewRequest("GET", "/api/earn/next?client_id=c1&user_id=u1", nil))
	var next2 map[string]any
	json.NewDecoder(rr.Body).Decode(&next2)
	if next2["id"] == taskID {
		t.Fatalf("expected new task after streak, still %v", taskID)
	}
}
