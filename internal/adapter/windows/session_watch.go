//go:build windows

package windows

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unsafe"

	"github.com/aegis/parental-control/internal/usecase/client"
)

const (
	wtsActive       = 0
	wtsConnected    = 1
	wtsConnectQuery = 2
	wtsShadow       = 3
	wtsDisconnected = 4
	wtsIdle         = 5
)

// ListSessions returns interactive sessions keyed by session ID.
func ListSessions() (map[uint32]client.SessionSnapshot, error) {
	ids, err := enumerateSessions()
	if err != nil {
		return nil, err
	}

	out := make(map[uint32]client.SessionSnapshot)
	for _, sid := range ids {
		if sid == 0 {
			continue
		}
		uname, err := getSessionUsername(sid)
		if err != nil || uname == "" {
			continue
		}
		namePart := uname
		if idx := strings.Index(uname, "\\"); idx >= 0 {
			namePart = uname[idx+1:]
		}
		// Skip clearly non-user accounts
		if strings.EqualFold(namePart, "SYSTEM") || strings.EqualFold(namePart, "LOCAL SERVICE") || strings.EqualFold(namePart, "NETWORK SERVICE") {
			continue
		}

		state := client.SessionActive // default: if we have a username, treat as present
		if rawState, err := getSessionConnectState(sid); err == nil {
			switch rawState {
			case wtsActive, wtsConnected, wtsConnectQuery, wtsShadow:
				state = client.SessionActive
			case wtsDisconnected, wtsIdle:
				state = client.SessionDisconnected
			default:
				// Keep Active default for unknown states with a real username
				state = client.SessionActive
			}
		}

		out[sid] = client.SessionSnapshot{
			SessionID: sid,
			Username:  namePart,
			State:     state,
			LogonTime: getSessionLogonTime(sid),
		}
	}
	return out, nil
}

func getSessionConnectState(sessionID uint32) (uint32, error) {
	var infoPtr uintptr
	var bytes uint32
	r1, _, err := procWTSQuerySessionInformationW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionID),
		WTSConnectState,
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 {
		return 0, err
	}
	defer procWTSFreeMemory.Call(infoPtr)
	if infoPtr == 0 || bytes < 4 {
		return 0, fmt.Errorf("no connect state")
	}
	return *(*uint32)(unsafe.Pointer(infoPtr)), nil
}

func getSessionLogonTime(sessionID uint32) time.Time {
	var infoPtr uintptr
	var bytes uint32
	r1, _, _ := procWTSQuerySessionInformationW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionID),
		WTSLogonTime,
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 || infoPtr == 0 || bytes < 8 {
		if infoPtr != 0 {
			procWTSFreeMemory.Call(infoPtr)
		}
		return time.Time{}
	}
	defer procWTSFreeMemory.Call(infoPtr)
	ft := *(*int64)(unsafe.Pointer(infoPtr))
	return filetimeToTime(ft)
}

func filetimeToTime(ft int64) time.Time {
	if ft <= 0 {
		return time.Time{}
	}
	const epochDiff = int64(116444736000000000)
	nsec := (ft - epochDiff) * 100
	if nsec <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nsec).UTC()
}

// SessionStateString for logging.
func SessionStateString(s client.SessionState) string {
	switch s {
	case client.SessionActive:
		return "active"
	case client.SessionDisconnected:
		return "disconnected"
	default:
		return fmt.Sprintf("other(%d)", s)
	}
}

// LogSessions writes a compact session summary (for diagnostics).
func LogSessions(sessions map[uint32]client.SessionSnapshot) {
	if len(sessions) == 0 {
		log.Printf("activity: no interactive sessions")
		return
	}
	parts := make([]string, 0, len(sessions))
	for sid, s := range sessions {
		lt := "-"
		if !s.LogonTime.IsZero() {
			lt = s.LogonTime.Local().Format("15:04:05")
		}
		parts = append(parts, fmt.Sprintf("%d=%s/%s@%s", sid, s.Username, SessionStateString(s.State), lt))
	}
	log.Printf("activity: sessions %s", strings.Join(parts, ", "))
}
