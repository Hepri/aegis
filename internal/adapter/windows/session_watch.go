//go:build windows

package windows

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/aegis/parental-control/internal/usecase/client"
)

const (
	wtsActive       = 0
	wtsConnected    = 1
	wtsDisconnected = 4
)

// ListSessions returns interactive sessions keyed by session ID.
func ListSessions() (map[uint32]client.SessionSnapshot, error) {
	var infoPtr uintptr
	var count uint32
	r1, _, err := procWTSEnumerateSessionsW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		0,
		1,
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 {
		return nil, err
	}
	defer procWTSFreeMemory.Call(infoPtr)
	if infoPtr == 0 {
		return map[uint32]client.SessionSnapshot{}, nil
	}

	out := make(map[uint32]client.SessionSnapshot)
	offset := infoPtr
	for i := uint32(0); i < count; i++ {
		si := (*wtsSessionInfo)(unsafe.Pointer(offset))
		sid := si.SessionID
		offset += unsafe.Sizeof(wtsSessionInfo{})
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
		state := client.SessionOther
		switch si.State {
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
		}
	}
	return out, nil
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
