//go:build windows

package windows

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
	"unsafe"

	"github.com/aegis/parental-control/internal/usecase/client"
	"golang.org/x/sys/windows"
)

const SessionAgentPipeName = `\\.\pipe\aegis-session-agent`

var (
	procWTSQueryUserToken    = wtsapi32.NewProc("WTSQueryUserToken")
	advapi32                 = windows.NewLazySystemDLL("advapi32.dll")
	procCreateProcessAsUserW = advapi32.NewProc("CreateProcessAsUserW")
	procDuplicateTokenEx     = advapi32.NewProc("DuplicateTokenEx")
)

const (
	securityImpersonation = 2
	tokenPrimary          = 1
	createUnicodeEnv      = 0x00000400
	createNoWindow        = 0x08000000
	stillActive           = 259
)

// SessionAgentManager launches per-session helpers and receives app snapshots over a named pipe.
type SessionAgentManager struct {
	mu       sync.Mutex
	exePath  string
	agents   map[uint32]uint32 // sessionID -> pid
	latest   map[uint32]client.AppWatchState
	latestMu sync.RWMutex
	stopCh   chan struct{}
}

func NewSessionAgentManager(exePath string) *SessionAgentManager {
	return &SessionAgentManager{
		exePath: exePath,
		agents:  make(map[uint32]uint32),
		latest:  make(map[uint32]client.AppWatchState),
		stopCh:  make(chan struct{}),
	}
}

func (m *SessionAgentManager) Start() error {
	go m.pipeServerLoop()
	return nil
}

func (m *SessionAgentManager) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}

// SyncAgents ensures an agent is running for each Active session.
func (m *SessionAgentManager) SyncAgents(sessions map[uint32]client.SessionSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for sid, snap := range sessions {
		if snap.State != client.SessionActive {
			continue
		}
		if pid, ok := m.agents[sid]; ok && processAlive(pid) {
			continue
		}
		pid, err := launchSessionAgent(m.exePath, sid, snap.Username)
		if err != nil {
			log.Printf("session-agent: launch for session %d (%s): %v", sid, snap.Username, err)
			continue
		}
		m.agents[sid] = pid
		log.Printf("session-agent: started pid=%d for session %d (%s)", pid, sid, snap.Username)
	}

	for sid := range m.agents {
		snap, ok := sessions[sid]
		if !ok || snap.State != client.SessionActive || !processAlive(m.agents[sid]) {
			delete(m.agents, sid)
			m.latestMu.Lock()
			delete(m.latest, sid)
			m.latestMu.Unlock()
		}
	}
}

// LatestStates returns the most recent app snapshot per session.
func (m *SessionAgentManager) LatestStates() map[uint32]client.AppWatchState {
	m.latestMu.RLock()
	defer m.latestMu.RUnlock()
	out := make(map[uint32]client.AppWatchState, len(m.latest))
	for k, v := range m.latest {
		out[k] = v
	}
	return out
}

type agentMessage struct {
	SessionID uint32               `json:"session_id"`
	Username  string               `json:"username"`
	State     client.AppWatchState `json:"state"`
}

func (m *SessionAgentManager) pipeServerLoop() {
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}
		pipe, err := createPipeServer(SessionAgentPipeName)
		if err != nil {
			log.Printf("session-agent: create pipe: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		err = windows.ConnectNamedPipe(pipe, nil)
		if err != nil && err != windows.ERROR_PIPE_CONNECTED {
			windows.CloseHandle(pipe)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		go m.handlePipeClient(pipe)
	}
}

func (m *SessionAgentManager) handlePipeClient(pipe windows.Handle) {
	defer windows.CloseHandle(pipe)
	r := &pipeFile{h: pipe}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var msg agentMessage
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}
		msg.State.Username = msg.Username
		m.latestMu.Lock()
		m.latest[msg.SessionID] = msg.State
		m.latestMu.Unlock()
	}
}

type pipeFile struct{ h windows.Handle }

func (p *pipeFile) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(p.h, b, &n, nil)
	if n > 0 {
		return int(n), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, io.EOF
}

func createPipeServer(name string) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	// NULL DACL would be ideal; InheritHandle + default SD often works for local SYSTEM pipe.
	h, err := windows.CreateNamedPipe(
		namePtr,
		windows.PIPE_ACCESS_INBOUND,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		255,
		0,
		64*1024,
		0,
		nil,
	)
	return h, err
}

func processAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

func launchSessionAgent(exePath string, sessionID uint32, username string) (uint32, error) {
	var userToken windows.Handle
	r1, _, err := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&userToken)))
	if r1 == 0 {
		return 0, fmt.Errorf("WTSQueryUserToken: %v", err)
	}
	defer windows.CloseHandle(userToken)

	var primary windows.Handle
	r1, _, err = procDuplicateTokenEx.Call(
		uintptr(userToken),
		windows.MAXIMUM_ALLOWED,
		0,
		securityImpersonation,
		tokenPrimary,
		uintptr(unsafe.Pointer(&primary)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("DuplicateTokenEx: %v", err)
	}
	defer windows.CloseHandle(primary)

	cmdLine := fmt.Sprintf(`"%s" session-agent --session-id=%d --username=%s`, exePath, sessionID, windows.EscapeArg(username))

	cmdPtr, err := windows.UTF16PtrFromString(cmdLine)
	if err != nil {
		return 0, err
	}
	appPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return 0, err
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop, _ = windows.UTF16PtrFromString(`winsta0\default`)
	var pi windows.ProcessInformation

	r1, _, err = procCreateProcessAsUserW.Call(
		uintptr(primary),
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(cmdPtr)),
		0, 0, 0,
		createUnicodeEnv|createNoWindow,
		0, 0,
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("CreateProcessAsUser: %v", err)
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return pi.ProcessId, nil
}

// RunSessionAgent is the entrypoint for the per-user helper process.
func RunSessionAgent(sessionID uint32, username string) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	connect := func() (windows.Handle, error) {
		namePtr, err := windows.UTF16PtrFromString(SessionAgentPipeName)
		if err != nil {
			return 0, err
		}
		return windows.CreateFile(
			namePtr,
			windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			0,
			0,
		)
	}

	var pipe windows.Handle
	for {
		h, err := connect()
		if err == nil {
			pipe = h
			break
		}
		time.Sleep(2 * time.Second)
	}
	defer windows.CloseHandle(pipe)

	send := func() {
		state := CaptureAppState(username)
		msg := agentMessage{SessionID: sessionID, Username: username, State: state}
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		data = append(data, '\n')
		var written uint32
		_ = windows.WriteFile(pipe, data, &written, nil)
	}

	send()
	for range ticker.C {
		send()
	}
}
