//go:build !windows

package windows

import (
	"github.com/aegis/parental-control/internal/domain"
	"github.com/aegis/parental-control/internal/usecase/client"
)

func ListSessions() (map[uint32]client.SessionSnapshot, error) {
	return map[uint32]client.SessionSnapshot{}, nil
}

type SessionAgentManager struct{}

func NewSessionAgentManager(exePath string) *SessionAgentManager { return &SessionAgentManager{} }
func (m *SessionAgentManager) Start() error                      { return nil }
func (m *SessionAgentManager) Stop()                             {}
func (m *SessionAgentManager) SyncAgents(sessions map[uint32]client.SessionSnapshot) {
}
func (m *SessionAgentManager) LatestStates() map[uint32]client.AppWatchState {
	return nil
}

func RunSessionAgent(sessionID uint32, username string) {}

func ApplyUpdateIfNeeded(localVersion string, update *domain.ClientUpdate, serverURL string) {}
