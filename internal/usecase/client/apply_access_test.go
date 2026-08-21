package client

import (
	"testing"
	"time"

	"github.com/aegis/parental-control/internal/domain"
)

type fakeCtrl struct {
	passwords map[string]string
	logoffs   []string
}

func (f *fakeCtrl) SetPassword(username, password string) error {
	if f.passwords == nil {
		f.passwords = map[string]string{}
	}
	f.passwords[username] = password
	return nil
}

func (f *fakeCtrl) DisconnectUserSession(username string) error {
	f.logoffs = append(f.logoffs, username)
	return nil
}

func TestApplyAccess_FirstPollBlocksOutsideInterval(t *testing.T) {
	ctrl := &fakeCtrl{}
	now := time.Date(2026, 8, 21, 23, 0, 0, 0, time.UTC)
	cfg := &domain.ClientConfig{
		Users: []domain.UserAccessConfig{{
			Username: "kid",
			AllowedIntervals: []domain.AllowedInterval{{
				Start: time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
			}},
		}},
	}
	state := ApplyAccessIfNeeded(ctrl, cfg, now, nil)
	if state["kid"] != false {
		t.Fatalf("expected blocked, got %v", state["kid"])
	}
	if len(ctrl.logoffs) != 1 || ctrl.logoffs[0] != "kid" {
		t.Fatalf("expected logoff, got %v", ctrl.logoffs)
	}
	if ctrl.passwords["kid"] == "" || ctrl.passwords["kid"] == "123456" {
		t.Fatalf("expected random lock password, got %q", ctrl.passwords["kid"])
	}
}

func TestLockUsers_BlocksAll(t *testing.T) {
	ctrl := &fakeCtrl{}
	state := LockUsers(ctrl, []string{"kid", "admin"})
	if state["kid"] != false || state["admin"] != false {
		t.Fatalf("expected all blocked, got %v", state)
	}
	if ctrl.passwords["kid"] == "" || ctrl.passwords["kid"] == unlockPassword {
		t.Fatalf("expected random lock password for kid, got %q", ctrl.passwords["kid"])
	}
	if len(ctrl.logoffs) != 2 {
		t.Fatalf("expected 2 logoffs, got %v", ctrl.logoffs)
	}
}

func TestApplyAccess_SkipsWhenUnchanged(t *testing.T) {
	ctrl := &fakeCtrl{}
	now := time.Date(2026, 8, 21, 23, 0, 0, 0, time.UTC)
	cfg := &domain.ClientConfig{
		Users: []domain.UserAccessConfig{{
			Username:         "kid",
			AllowedIntervals: nil,
		}},
	}
	prev := map[string]bool{"kid": false}
	ApplyAccessIfNeeded(ctrl, cfg, now, prev)
	if len(ctrl.logoffs) != 0 {
		t.Fatalf("should not re-logoff: %v", ctrl.logoffs)
	}
}
