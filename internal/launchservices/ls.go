// Package launchservices asks macOS Launch Services which apps open a file or
// a type, without cgo. The types here build on every OS so that callers can
// be tested with a fake; only darwin has an implementation.
package launchservices

// Role is a Launch Services role mask (kLSRoles…): what an app may do with a
// type.
type Role uint32

const (
	// Viewer apps can open a file to read it (kLSRolesViewer).
	Viewer Role = 0x2
	// Editor apps can open a file and change it (kLSRolesEditor).
	Editor Role = 0x4
)

// Bundle is an application bundle.
type Bundle struct {
	// ID is the bundle identifier, such as "com.apple.TextEdit"; it may be
	// empty for a bundle that declares none.
	ID string
	// Name is the bundle's display name.
	Name string
	// Path is the absolute path of the .app folder.
	Path string
}

// Services answers what Launch Services knows about apps for files and types.
type Services interface {
	// ForFile lists the apps that can open path in role r.
	ForFile(path string, r Role) ([]Bundle, error)
	// DefaultForFile is the app Launch Services would use for path in role r;
	// ok is false when there is none.
	DefaultForFile(path string, r Role) (b Bundle, ok bool, err error)
	// ForType lists the apps that handle the extension ext (with or without
	// its dot) in role r.
	ForType(ext string, r Role) ([]Bundle, error)
	// DefaultForType is the app that handles ext in role r by default; ok is
	// false when there is none.
	DefaultForType(ext string, r Role) (b Bundle, ok bool, err error)
}
