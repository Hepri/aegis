//go:build windows

package windows

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/aegis/parental-control/internal/usecase/client"
	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	psapi                        = windows.NewLazySystemDLL("psapi.dll")
	versionDLL                   = windows.NewLazySystemDLL("version.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindow                = user32.NewProc("GetWindow")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetFileVersionInfoSizeW  = versionDLL.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW      = versionDLL.NewProc("GetFileVersionInfoW")
	procVerQueryValueW           = versionDLL.NewProc("VerQueryValueW")
)

const (
	gwOwner            = 4
	processQueryLimited = 0x1000
)

var ignoredExeNames = map[string]bool{
	"dwm.exe":              true,
	"csrss.exe":            true,
	"winlogon.exe":         true,
	"fontdrvhost.exe":      true,
	"sihost.exe":           true,
	"taskhostw.exe":        true,
	"runtimebroker.exe":    true,
	"applicationframehost.exe": true,
	"systemsettings.exe":   true,
	"textinputhost.exe":    true,
	"searchhost.exe":       true,
	"startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true,
	"lockapp.exe":          true,
	"logonui.exe":          true,
	"explorer.exe":         false, // keep explorer (desktop/shell can be useful)
	"aegis-client.exe":     true,
}

// CaptureAppState snapshots visible windows and foreground app in the current session.
func CaptureAppState(username string) client.AppWatchState {
	appsByExe := make(map[string]client.AppSnapshot)
	enumVisibleWindows(func(hwnd windows.HWND) {
		if !isTopLevelVisible(hwnd) {
			return
		}
		pid := windowPID(hwnd)
		if pid == 0 {
			return
		}
		exe := processExePath(pid)
		if exe == "" {
			return
		}
		base := strings.ToLower(filepath.Base(exe))
		if ignored, ok := ignoredExeNames[base]; ok && ignored {
			return
		}
		name := fileDescription(exe)
		if name == "" {
			name = filepath.Base(exe)
		}
		appsByExe[strings.ToLower(exe)] = client.AppSnapshot{AppName: name, ExePath: exe}
	})

	apps := make([]client.AppSnapshot, 0, len(appsByExe))
	for _, a := range appsByExe {
		apps = append(apps, a)
	}

	var focused *client.AppSnapshot
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd != 0 {
		pid := windowPID(windows.HWND(hwnd))
		exe := processExePath(pid)
		if exe != "" {
			base := strings.ToLower(filepath.Base(exe))
			if ignored, ok := ignoredExeNames[base]; !ok || !ignored {
				name := fileDescription(exe)
				if name == "" {
					name = filepath.Base(exe)
				}
				f := client.AppSnapshot{AppName: name, ExePath: exe}
				focused = &f
				appsByExe[strings.ToLower(exe)] = f
				// refresh apps slice if we added focus-only
				found := false
				for _, a := range apps {
					if strings.EqualFold(a.ExePath, exe) {
						found = true
						break
					}
				}
				if !found {
					apps = append(apps, f)
				}
			}
		}
	}

	return client.AppWatchState{Username: username, Apps: apps, Focused: focused}
}

func isTopLevelVisible(hwnd windows.HWND) bool {
	r, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	if r == 0 {
		return false
	}
	owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner)
	if owner != 0 {
		return false
	}
	lenR, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	return lenR > 0
}

func windowPID(hwnd windows.HWND) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	return pid
}

type enumProc = func(hwnd windows.HWND)

var enumCallback enumProc

func enumVisibleWindows(fn enumProc) {
	enumCallback = fn
	cb := syscall.NewCallback(func(hwnd windows.HWND, lparam uintptr) uintptr {
		if enumCallback != nil {
			enumCallback(hwnd)
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	enumCallback = nil
}

func processExePath(pid uint32) string {
	h, _, _ := procOpenProcess.Call(processQueryLimited, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func fileDescription(exePath string) string {
	pathPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(pathPtr)), 0)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	r, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, size, uintptr(unsafe.Pointer(&buf[0])))
	if r == 0 {
		return ""
	}
	// Try translation-independent query via \StringFileInfo\040904B0\FileDescription then fall back
	for _, lang := range []string{`\StringFileInfo\040904B0\FileDescription`, `\StringFileInfo\040904E4\FileDescription`, `\StringFileInfo\000004B0\FileDescription`} {
		if s := verQueryString(buf, lang); s != "" {
			return s
		}
	}
	// Enumerate first translation
	var transPtr uintptr
	var transLen uint32
	subBlock, _ := windows.UTF16PtrFromString(`\VarFileInfo\Translation`)
	r, _, _ = procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(subBlock)), uintptr(unsafe.Pointer(&transPtr)), uintptr(unsafe.Pointer(&transLen)))
	if r != 0 && transLen >= 4 {
		lang := *(*uint16)(unsafe.Pointer(transPtr))
		codepage := *(*uint16)(unsafe.Pointer(transPtr + 2))
		key := `\StringFileInfo\` + toHex4(lang) + toHex4(codepage) + `\FileDescription`
		if s := verQueryString(buf, key); s != "" {
			return s
		}
	}
	return ""
}

func verQueryString(buf []byte, key string) string {
	subBlock, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return ""
	}
	var valPtr uintptr
	var valLen uint32
	r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(subBlock)), uintptr(unsafe.Pointer(&valPtr)), uintptr(unsafe.Pointer(&valLen)))
	if r == 0 || valPtr == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(unsafe.Pointer(valPtr))))
}

func toHex4(v uint16) string {
	const hexdigits = "0123456789ABCDEF"
	return string([]byte{
		hexdigits[(v>>12)&0xf],
		hexdigits[(v>>8)&0xf],
		hexdigits[(v>>4)&0xf],
		hexdigits[v&0xf],
	})
}
