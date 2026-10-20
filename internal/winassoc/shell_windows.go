package winassoc

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
	shlwapi             = windows.NewLazySystemDLL("shlwapi.dll")
	procAssocQueryStrW  = shlwapi.NewProc("AssocQueryStringW")
)

const (
	assocstrFriendlyAppName = 4 // ASSOCSTR_FRIENDLYAPPNAME

	seeMaskClassName = 0x00000001 // SEE_MASK_CLASSNAME
	seeMaskNoAsync   = 0x00000100 // SEE_MASK_NOASYNC
)

// shellExecuteInfo is SHELLEXECUTEINFOW, which x/sys does not declare. Go
// lays it out as C does on every Windows architecture.
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

// FriendlyName is the name Windows shows for progID's verb, such as
// "Paint"; it is progID itself when Windows has none.
func FriendlyName(progID, verb string) string {
	assoc, err := windows.UTF16PtrFromString(progID)
	if err != nil {
		return progID
	}
	extra, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return progID
	}
	// The first call measures, the second fills.
	var n uint32
	r, _, _ := procAssocQueryStrW.Call(0, assocstrFriendlyAppName,
		uintptr(unsafe.Pointer(assoc)), uintptr(unsafe.Pointer(extra)), 0, uintptr(unsafe.Pointer(&n)))
	if (r != 0 && r != 1) || n == 0 || n > 32768 { // S_OK or S_FALSE
		return progID
	}
	buf := make([]uint16, n)
	r, _, _ = procAssocQueryStrW.Call(0, assocstrFriendlyAppName,
		uintptr(unsafe.Pointer(assoc)), uintptr(unsafe.Pointer(extra)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return progID
	}
	if name := windows.UTF16ToString(buf); name != "" {
		return name
	}
	return progID
}

// ShellExecute asks the shell to perform verb on path: with the command
// that progID registers, or when progID is "" with the type's own handler.
// path is passed as the file, never through a command line.
func ShellExecute(path, verb, progID string) error {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verbp, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		fMask:  seeMaskNoAsync,
		lpVerb: verbp,
		lpFile: file,
		nShow:  windows.SW_SHOWNORMAL,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if progID != "" {
		if info.lpClass, err = windows.UTF16PtrFromString(progID); err != nil {
			return err
		}
		info.fMask |= seeMaskClassName
	}

	// The shell may run extensions that need COM on the calling thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); {
	case err == nil, errors.Is(err, windows.Errno(windows.S_FALSE)):
		defer windows.CoUninitialize()
	}

	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(file)
	runtime.KeepAlive(verbp)
	if ok == 0 {
		if callErr == nil || callErr == windows.ERROR_SUCCESS {
			return errors.New("ShellExecuteEx failed")
		}
		return callErr
	}
	return nil
}
