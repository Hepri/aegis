package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aegis/parental-control/internal/domain"
)

func TestManagedUsersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := SaveManagedUsers(dir, []string{"sasha", "admin"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManagedUsers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "sasha" || got[1] != "admin" {
		t.Fatalf("got %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "managed-users.json")); err != nil {
		t.Fatal(err)
	}
}

func TestUsernamesFromConfig(t *testing.T) {
	cfg := &domain.ClientConfig{
		Users: []domain.UserAccessConfig{
			{Username: "a"},
			{Username: "a"},
			{Username: "b"},
			{Username: ""},
		},
	}
	got := UsernamesFromConfig(cfg)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
}
