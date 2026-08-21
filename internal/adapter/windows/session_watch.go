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
	wtsDisconnected = 4
)

// ListSessions returns interactive sessions keyed by session ID.
// Connect state and logon time come from WTSQuerySessionInformation (not the
// enumerate struct layout), so login timestamps match the real Windows logon.
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
		rawState, err := getSessionConnectState(sid)
		if err != nil {
			continue
		}
		state := client.SessionOther
		switch rawState {
		case wtsActive, wtsConnected:
			state = client.SessionActive
		case wtsDisconnected:
			state = client.SessionDisconnected
		default:
			continue
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
	// Prefer WTSINFO.LogonTime (WTSSessionInfo = 24)
	var infoPtr uintptr
	var bytes uint32
	r1, _, _ := procWTSQuerySessionInformationW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionID),
		WTSSessionInfo,
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 != 0 && infoPtr != 0 && bytes >= 72 {
		// WTSINFO layout (x64): State(4)+pad(4)+SessionId(4)+IncomingBytes(4)+
		// OutgoingBytes(4)+IncomingFrames(4)+OutgoingFrames(4)+IncomingCompressed(4)+
		// OutgoingCompressed(4) = 36, then pad to 40 for LARGE_INTEGER alignment,
		// then IdleTime(8)+LastInput(8? no IdleTime is LARGE_INTEGER at offset...)
		// Actual WTSINFO (Windows SDK):
		//   State DWORD + SessionId DWORD + IncomingBytes DWORD + OutgoingBytes DWORD
		//   IncomingFrames DWORD + OutgoingFrames DWORD + IncomingCompressedBytes DWORD
		//   OutgoingCompressedBytes DWORD = 32 bytes
		//   WinStationName WCHAR[32] = 64 -> total 96
		//   Domain WCHAR[17] ... this gets messy across SDK versions.
		//
		// Safer: use WTSLogonTime (18) which returns a pointer to a 64-bit FILETIME/LARGE_INTEGER.
		procWTSFreeMemory.Call(infoPtr)
	} else if infoPtr != 0 {
		procWTSFreeMemory.Call(infoPtr)
	}

	r1, _, _ = procWTSQuerySessionInformationW.Call(
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
	// FILETIME: 100ns since 1601-01-01 UTC. Unix epoch offset:
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
