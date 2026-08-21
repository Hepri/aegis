//go:build windows

package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/aegis/parental-control/internal/usecase/client"
	"golang.org/x/sys/windows"
)

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
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
	detachedProcess       = 0x00000008
	startfUseShowWindow   = 0x00000001
	swHide                = 0
	stillActive           = 259
)

func agentStateDir() string {
	// Shared location both SYSTEM service and user session can use.
	dir := filepath.Join(os.Getenv("ProgramData"), "Aegis", "agent")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func agentStatePath(sessionID uint32) string {
	return filepath.Join(agentStateDir(), fmt.Sprintf("session-%d.json", sessionID))
}

// SessionAgentManager launches per-session helpers; they write app snapshots to ProgramData JSON files.
type SessionAgentManager struct {
	mu      sync.Mutex
	exePath string
	agents  map[uint32]uint32 // sessionID -> pid
	stopCh  chan struct{}
}

func NewSessionAgentManager(exePath string) *SessionAgentManager {
	return &SessionAgentManager{
		exePath: exePath,
		agents:  make(map[uint32]uint32),
		stopCh:  make(chan struct{}),
	}
}

func (m *SessionAgentManager) Start() error {
	_ = os.MkdirAll(agentStateDir(), 0755)
	// Previous service runs leave session-agent processes behind; clear them before relaunch.
	killed := killOtherClientProcesses(m.exePath)
	if killed > 0 {
		log.Printf("session-agent: killed %d leftover process(es) on start", killed)
	}
	clearAgentStateFiles()
	return nil
}

func (m *SessionAgentManager) Stop() {
	m.mu.Lock()
	for sid, pid := range m.agents {
		killProcess(pid)
		delete(m.agents, sid)
		_ = os.Remove(agentStatePath(sid))
	}
	m.mu.Unlock()
	killed := killOtherClientProcesses(m.exePath)
	if killed > 0 {
		log.Printf("session-agent: killed %d leftover process(es) on stop", killed)
	}
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
		if old, ok := m.agents[sid]; ok {
			killProcess(old)
		}
		pid, err := launchSessionAgent(m.exePath, sid, snap.Username)
		if err != nil {
			log.Printf("session-agent: launch for session %d (%s): %v", sid, snap.Username, err)
			continue
		}
		m.agents[sid] = pid
		log.Printf("session-agent: started pid=%d for session %d (%s)", pid, sid, snap.Username)
	}

	for sid, pid := range m.agents {
		snap, ok := sessions[sid]
		if ok && snap.State == client.SessionActive && processAlive(pid) {
			continue
		}
		killProcess(pid)
		delete(m.agents, sid)
		_ = os.Remove(agentStatePath(sid))
	}
}

// LatestStates reads the most recent app snapshot per session from disk.
func (m *SessionAgentManager) LatestStates() map[uint32]client.AppWatchState {
	out := make(map[uint32]client.AppWatchState)
	m.mu.Lock()
	sids := make([]uint32, 0, len(m.agents))
	for sid := range m.agents {
		sids = append(sids, sid)
	}
	m.mu.Unlock()

	for _, sid := range sids {
		data, err := os.ReadFile(agentStatePath(sid))
		if err != nil {
			continue
		}
		var msg agentMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		// Ignore stale files (>2 min old)
		if !msg.WrittenAt.IsZero() && time.Since(msg.WrittenAt) > 2*time.Minute {
			continue
		}
		msg.State.Username = msg.Username
		out[sid] = msg.State
	}
	return out
}

type agentMessage struct {
	SessionID uint32               `json:"session_id"`
	Username  string               `json:"username"`
	WrittenAt time.Time            `json:"written_at"`
	State     client.AppWatchState `json:"state"`
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

func killProcess(pid uint32) {
	if pid == 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.TerminateProcess(h, 1)
}

// killOtherClientProcesses terminates every process with the same exe name except the current one.
// Session-agents share aegis-client.exe; after service restart they would otherwise orphan.
func killOtherClientProcesses(exePath string) int {
	selfPID := uint32(os.Getpid())
	want := strings.ToLower(filepath.Base(exePath))
	if want == "" {
		return 0
	}

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		log.Printf("session-agent: process snapshot: %v", err)
		return 0
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return 0
	}
	killed := 0
	for {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if name == want && pe.ProcessID != selfPID {
			log.Printf("session-agent: terminating leftover pid=%d", pe.ProcessID)
			killProcess(pe.ProcessID)
			killed++
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return killed
}

func clearAgentStateFiles() {
	dir := agentStateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".json") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
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
	si.Flags = startfUseShowWindow
	si.ShowWindow = swHide
	var pi windows.ProcessInformation

	// Hidden helper in the user session — no console window the child can close.
	r1, _, err = procCreateProcessAsUserW.Call(
		uintptr(primary),
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(cmdPtr)),
		0, 0, 0,
		createUnicodeEnv|createNewProcessGroup|createNoWindow|detachedProcess,
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
	hideAgentConsole()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logPath := filepath.Join(agentStateDir(), fmt.Sprintf("agent-%d.log", sessionID))
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.Printf("session-agent starting session=%d user=%s", sessionID, username)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	write := func() {
		state := CaptureAppState(username)
		msg := agentMessage{
			SessionID: sessionID,
			Username:  username,
			WrittenAt: time.Now(),
			State:     state,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		path := agentStatePath(sessionID)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0666); err != nil {
			log.Printf("write state: %v", err)
			return
		}
		_ = os.Rename(tmp, path)
		nApps := len(state.Apps)
		focus := "-"
		if state.Focused != nil {
			focus = state.Focused.AppName
		}
		log.Printf("snapshot apps=%d focus=%s", nApps, focus)
	}

	write()
	for range ticker.C {
		write()
	}
}

func HideAgentConsole() {
	hideAgentConsole()
}

func hideAgentConsole() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	user32 := windows.NewLazySystemDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	freeConsole := kernel32.NewProc("FreeConsole")
	showWindow := user32.NewProc("ShowWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, swHide)
	}
	freeConsole.Call()
}
