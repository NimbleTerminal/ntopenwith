package winassoc

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSHLoadIndirectString = shlwapi.NewProc("SHLoadIndirectString")

// FriendlyName is the name Windows shows for progID's verb, such as
// "Paint". It tries the shell, ApplicationName, and the executable's file
// description or base name before falling back to progID.
func FriendlyName(progID, verb string) string {
	return friendlyName(progID, verb, nameLookups{
		query: queryAssociation,
		application: func(class string) string {
			name, _ := OS().String("HKCR", class+`\Application`, "ApplicationName")
			return name
		},
		indirect:    loadIndirectString,
		description: fileDescription,
	})
}

func loadIndirectString(s string) string {
	source, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 32768)
	r, _, _ := procSHLoadIndirectString.Call(uintptr(unsafe.Pointer(source)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if r != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func fileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 || size > 16<<20 {
		return ""
	}
	data := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])); err != nil {
		return ""
	}
	// VerQueryValue returns pointers into data, which must stay alive until
	// the description has been copied into a Go string.
	defer func() { runtime.KeepAlive(data) }()
	var translation *[2]uint16
	var n uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&data[0]), `\VarFileInfo\Translation`,
		unsafe.Pointer(&translation), &n); err != nil || translation == nil || n < 4 {
		return ""
	}
	key := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, translation[0], translation[1])
	var description *uint16
	if err := windows.VerQueryValue(unsafe.Pointer(&data[0]), key,
		unsafe.Pointer(&description), &n); err != nil || description == nil || n == 0 || n > 32768 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice(description, n))
}
