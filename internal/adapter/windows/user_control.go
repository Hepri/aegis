//go:build windows

package windows

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wtsapi32                        = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSEnumerateSessionsW       = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInformationW = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSDisconnectSession        = wtsapi32.NewProc("WTSDisconnectSession")
	procWTSLogoffSession            = wtsapi32.NewProc("WTSLogoffSession")
	procWTSFreeMemory               = wtsapi32.NewProc("WTSFreeMemory")
)

const (
	WTS_CURRENT_SERVER_HANDLE = 0
	WTSInitialProgram         = 0
	WTSApplicationName        = 1
	WTSWorkingDirectory       = 2
	WTSOEMId                  = 3
	WTSSessionId              = 4
	WTSUserName               = 5
	WTSWinStationName         = 6
	WTSDomainName             = 7
	WTSConnectState           = 8
	WTSClientBuildNumber      = 9
	WTSClientName             = 10
	WTSClientDirectory        = 11
	WTSClientProductId        = 12
	WTSClientHardwareId       = 13
	WTSClientAddress          = 14
	WTSClientDisplay          = 15
	WTSClientProtocolType     = 16
	WTSIdleTime               = 17
	WTSLogonTime              = 18
	WTSIncomingBytes          = 19
	WTSOutgoingBytes          = 20
	WTSIncomingFrames         = 21
	WTSOutgoingFrames         = 22
	WTSClientInfo             = 23
	WTSSessionInfo            = 24
	WTSSessionInfoEx          = 25
	WTSConfigInfo             = 26
	WTSValidationInfo         = 27
	WTSSessionAddressV4       = 28
	WTSIsRemoteSession        = 29
)

type UserControl struct{}

func NewUserControl() *UserControl {
	return &UserControl{}
}

func (u *UserControl) SetPassword(username, password string) error {
	cmd := exec.Command("net", "user", username, password)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	if err != nil {
		log.Printf("SetPassword %q failed: %v (try running as admin, local account only)", username, err)
	}
	return err
}

func (u *UserControl) DisconnectUserSession(username string) error {
	sessions, err := enumerateSessions()
	if err != nil {
		log.Printf("DisconnectUserSession: enumerate sessions failed: %v", err)
		return err
	}
	var lastErr error
	for _, sid := range sessions {
		if sid == 0 {
			continue // skip session 0 (services)
		}
		uname, err := getSessionUsername(sid)
		if err != nil {
			continue
		}
		// Match username or "DOMAIN\username"
		namePart := uname
		if idx := strings.Index(uname, "\\"); idx >= 0 {
			namePart = uname[idx+1:]
		}
		if strings.EqualFold(namePart, username) {
			if err := logoffSession(sid); err != nil {
				log.Printf("DisconnectUserSession: logoff session %d (%s) failed: %v", sid, uname, err)
				lastErr = err
			} else {
				log.Printf("DisconnectUserSession: logged off session %d (%s)", sid, uname)
			}
		}
	}
	return lastErr
}

// WTS_SESSION_INFO layout for 64-bit: SessionId(4) + padding(4) + pWinStationName(8) + State(4) + padding(4) = 24 bytes
type wtsSessionInfo struct {
	SessionID  uint32
	_          uint32 // padding for pointer alignment
	WinStation uintptr
	State      uint32
	_          uint32 // padding
}

func enumerateSessions() ([]uint32, error) {
	var infoPtr uintptr
	var count uint32
	r1, _, err := procWTSEnumerateSessionsW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		0, // reserved
		1, // version
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 {
		return nil, err
	}
	defer procWTSFreeMemory.Call(infoPtr)

	if infoPtr == 0 {
		return nil, nil
	}

	var sess []uint32
	offset := infoPtr
	for i := uint32(0); i < count; i++ {
		si := (*wtsSessionInfo)(unsafe.Pointer(offset))
		sess = append(sess, si.SessionID)
		offset += unsafe.Sizeof(wtsSessionInfo{})
	}
	return sess, nil
}

func getSessionUsername(sessionID uint32) (string, error) {
	var infoPtr uintptr
	var bytes uint32
	r1, _, err := procWTSQuerySessionInformationW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionID),
		WTSUserName,
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 {
		return "", err
	}
	defer procWTSFreeMemory.Call(infoPtr)
	if infoPtr == 0 {
		return "", fmt.Errorf("no username")
	}
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(infoPtr))), nil
}

func disconnectSession(sessionID uint32) error {
	r1, _, err := procWTSDisconnectSession.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionID),
		0,
	)
	if r1 == 0 {
		return err
	}
	return nil
}

func logoffSession(sessionID uint32) error {
	r1, _, err := procWTSLogoffSession.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionID),
		0, // wait for logoff to complete
	)
	if r1 == 0 {
		return err
	}
	return nil
}
