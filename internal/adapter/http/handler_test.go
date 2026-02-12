package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/adapter/jsonfile"
	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/port"
	"github.com/aegis/parental-control/internal/usecase/server"
)

type mockRepo struct {
	state *port.ClientState
}

func (m *mockRepo) GetClient(ctx context.Context, clientID string) (*port.ClientState, error) {
	if m.state != nil {
		return m.state, nil
	}
	return nil, nil
}

func (m *mockRepo) GetAllClients(ctx context.Context) ([]*port.ClientState, error) { return nil, nil }
func (m *mockRepo) SaveClient(ctx context.Context, client *port.ClientState) error {
	config, _ := server.ComputeClientConfig(time.Now(), client, true)
	client.ComputedConfig = &config
	m.state = client
	return nil
}
func (m *mockRepo) AddUser(ctx context.Context, clientID string, user domain.User) error { return nil }
func (m *mockRepo) UpdateUserSchedule(ctx context.Context, clientID, userID string, schedule domain.DaySchedule) error {
	return nil
}
func (m *mockRepo) DeleteUser(ctx context.Context, clientID, userID string) error { return nil }
func (m *mockRepo) DeleteClient(ctx context.Context, clientID string) error       { return nil }
func (m *mockRepo) GrantTemporaryAccess(ctx context.Context, clientID, userID string, until time.Time) error {
	return nil
}
func (m *mockRepo) BlockClient(ctx context.Context, clientID, userID string, start, until time.Time) error {
	return nil
}
func (m *mockRepo) DeleteBlockRequest(ctx context.Context, clientID, requestID string) error {
	return nil
}
func (m *mockRepo) DeleteTemporaryAccessRequest(ctx context.Context, clientID, requestID string) error {
	return nil
}
func (m *mockRepo) UpdateLastSent(ctx context.Context, clientID string, intervals map[string][]domain.AllowedInterval) error {
	return nil
}
func (m *mockRepo) IncrementConfigVersion(ctx context.Context, clientID string) error {
	return nil
}
func (m *mockRepo) Subscribe(ctx context.Context, clientID string) <-chan struct{} {
	ch := make(chan struct{}, 1)
	return ch
}

func TestServeConfig_NewClient(t *testing.T) {
	repo := &mockRepo{}
	handler := NewHandler(repo)

	// First, save the client
	clientState := &port.ClientState{
		ID:    "test-123",
		Name:  "Test Client",
		Users: []domain.User{},
	}
	if err := repo.SaveClient(context.Background(), clientState); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/config?client_id=test-123", nil)
	rr := httptest.NewRecorder()

	handler.ServeConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Error("expected non-empty body")
	}
}

func TestServeConfig_NonexistentClient(t *testing.T) {
	repo := &mockRepo{}
	handler := NewHandler(repo)

	req := httptest.NewRequest("GET", "/api/config?client_id=nonexistent", nil)
	rr := httptest.NewRecorder()

	handler.ServeConfig(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestServeConfig_WithJsonfile(t *testing.T) {
	repo, err := jsonfile.New(t.TempDir() + "/test.json")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(repo)

	// First, save the client
	clientState := &port.ClientState{
		ID:    "test-456",
		Name:  "Test Client 456",
		Users: []domain.User{},
	}
	if err := repo.SaveClient(context.Background(), clientState); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/config?client_id=test-456", nil)
	rr := httptest.NewRecorder()

	handler.ServeConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Error("expected non-empty body")
	}
}
