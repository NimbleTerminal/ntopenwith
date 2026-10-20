package ntopenwith

import (
	"context"
	"errors"
	"os/exec"
)

// IDs of the apps every platform can offer.
const (
	// SystemID is the operating system's default handler.
	SystemID = "system"
	// TerminalEditorID is the editor named by $VISUAL or $EDITOR.
	TerminalEditorID = "terminal-editor"
)

var (
	// ErrUnsupported means the source cannot do this on the current platform.
	ErrUnsupported = errors.New("not supported on this platform")
	// ErrChoose means no default app is set, so the person must choose one.
	ErrChoose = errors.New("no default app: choose one")
	// ErrNoEditor means no app can edit the file.
	ErrNoEditor = errors.New("no app can edit this file")
	// ErrNotOffered means the app is not offered for this file and verb.
	ErrNotOffered = errors.New("app is not offered for this file")
)

// App is one choice of program for a target.
type App struct {
	// ID names the app in preferences: "system", "terminal-editor",
	// "mac:<bundle id>", "xdg:<desktop file id>", "win:<ProgID>",
	// "win:app:<exe name>", or an ID of the program's own for a List.
	ID string
	// Name is what to show a person.
	Name string
	// Terminal is true when Launch returns a command that needs the terminal.
	Terminal bool
	// Default is true for the app the platform would use unasked.
	Default bool

	//lint:ignore U1000 set and read by Layered, which comes in a later change
	from Source // the source that offered it
}

// Source offers apps for a target and launches them.
type Source interface {
	// Apps lists the apps that can perform v on t, in the order to show them.
	Apps(ctx context.Context, t Target, v Verb) ([]App, error)
	// Launch starts app on t. A graphical app is started and a nil command
	// returned; a terminal app comes back as an unstarted command to run.
	Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error)
}
