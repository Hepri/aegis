package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

// EventUploader queues activity events locally and posts them to the server in batches.
type EventUploader struct {
	mu       sync.Mutex
	baseURL  string
	clientID string
	queuePath string
	client   *http.Client
}

func NewEventUploader(baseURL, clientID, queuePath string) *EventUploader {
	return &EventUploader{
		baseURL:   baseURL,
		clientID:  clientID,
		queuePath: queuePath,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (u *EventUploader) Enqueue(events []domain.ActivityEvent) error {
	if len(events) == 0 {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	f, err := os.OpenFile(u.queuePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}
	return nil
}

// Flush reads the queue and POSTs events; on success truncates the queue.
func (u *EventUploader) Flush(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	data, err := os.ReadFile(u.queuePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	var events []domain.ActivityEvent
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var ev domain.ActivityEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if len(events) == 0 {
		_ = os.WriteFile(u.queuePath, nil, 0644)
		return nil
	}

	body, _ := json.Marshal(map[string]any{"events": events})
	url := fmt.Sprintf("%s/api/clients/%s/events", u.baseURL, u.clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("events upload status %d", resp.StatusCode)
	}
	return os.WriteFile(u.queuePath, nil, 0644)
}
