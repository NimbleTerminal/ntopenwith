# ntopenwith

<p>
    <a href="https://github.com/NimbleTerminal/ntopenwith/tags"><img src="https://img.shields.io/github/tag/NimbleTerminal/ntopenwith.svg" alt="Latest Release"></a>
    <a href="https://pkg.go.dev/nimbleterminal.dev/ntopenwith"><img src="https://pkg.go.dev/badge/nimbleterminal.dev/ntopenwith.svg" alt="GoDoc"></a>
</p>

Open and edit files with the apps the system knows. For a file and a verb,
*open* or *edit*, ntopenwith lists the apps that can do it, picks one (the
operating system's default, unless the person has saved a choice of their own),
and launches it without a shell.

It has no user interface and no dependency on Bubble Tea: your program draws the
menu and stores the choice. It is the library behind
[gloss](https://github.com/NimbleTerminal/gloss)'s `w` and `W` keys, and knows
nothing of gloss.

Module `nimbleterminal.dev/ntopenwith`, Go 1.26.8+. No CGO: it builds for every
OS, and under `js/wasm` it lists nothing and every launch returns
`ErrUnsupported`.

```sh
go get nimbleterminal.dev/ntopenwith
```

To try it on your machine, from a clone:

```sh
task build
bin/ntopen list photo.png     # the apps for open and edit; * marks the default
bin/ntopen open photo.png
bin/ntopen edit notes.txt --app mac:com.apple.TextEdit
```

## Three sources

Every list of apps is a `Source`: `Apps` says which can act on a `Target` with a
`Verb`, and `Launch` starts one. Use one, or lay one on another.

### The system: `system.System()`

The operating system's own index of handlers, with its default first. On macOS
that is Launch Services (through purego, no CGO), where "edit" is a real role, so
Preview opens a PNG and Pixelmator edits it. On Linux and the BSDs it is the XDG
data: the MIME globs (`mime/globs2`), the desktop entries' `MimeType=`, and
`mimeapps.list`. On Windows it is
the registry and the shell. The app `system` ("System default") hands the file to
the OS's own opener and is offered for Open everywhere.

```go
import (
	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/system"
)

src := system.System()
apps, err := src.Apps(ctx, ntopenwith.Target{Path: "/home/me/photo.png"}, ntopenwith.Open)
// apps[0] is the OS default, when it has one. Names are data: escape them
// before drawing them in a terminal.
```

Listings are kept for five seconds; `system.Refresh()` drops them, for after an
install. Asking for a type alone (no `Path`, an `Ext` such as `.png`) works too,
as a settings screen needs. Launching needs a path.

### Your own list: `ntopenwith.List`

A fixed list that never reads the system, written in Go and compiled into your
program. No file or environment variable can add to it.

```go
src := ntopenwith.List(
	ntopenwith.ListApp{
		ID: "gimp", Name: "GIMP", Verbs: []ntopenwith.Verb{ntopenwith.Edit},
		Kinds:   []string{".png", ".jpg"},
		Command: []string{"gimp", "{}"}, // "{}" is the path, as a whole argument
		Default: []ntopenwith.Verb{ntopenwith.Edit},
	},
	ntopenwith.ListApp{
		ID: "preview", Name: "Preview", Verbs: []ntopenwith.Verb{ntopenwith.Open},
		Bundle: "Preview.app", // macOS: open -a
	},
)
```

An app is offered only while its program is installed (`Find`, by default the
command or the bundle), and is checked again at launch.
`ntopenwith.TerminalEditor()` is the editor named by `$VISUAL`, else `$EDITOR`
(an executable path, or POSIX-quoted words with no expansion, at most 8 KiB and 64
words); `ntopenwith.TerminalEditors()` is nvim, vim, hx, nano, micro and emacs, each
when it is on the `PATH`. Both are for Edit of text only, and are *terminal*
apps: `Launch` returns an unstarted `*exec.Cmd`, because only you can hand them
your terminal.

### Both: `ntopenwith.Layered`

Sources asked in order, merged by app ID, with the first default kept. Later
entries may correct the earlier ones:

```go
src := ntopenwith.Layered(
	system.System(),
	ntopenwith.HideFor([]ntopenwith.Verb{ntopenwith.Edit}, []string{".png", ".jpg"},
		"mac:com.apple.QuickTimePlayerX"), // not an image editor
	ntopenwith.Rename("mac:com.microsoft.VSCode", "VS Code"),
	ntopenwith.Rank("mac:com.microsoft.VSCode"),                // listed first
	ntopenwith.Alias("vscode", "mac:com.microsoft.VSCode"),     // an old saved ID still works
	ntopenwith.List(ntopenwith.TerminalEditor()),
	ntopenwith.List(ntopenwith.TerminalEditors()...),
)
```

`Hide(ids...)` drops apps for every verb and type; `HideFor(verbs, kinds,
ids...)` only for those verbs and kinds (either empty means all). A `ListApp`
with the ID of an app already listed only renames it. `Override(src)` lets a
later source's default win. `Layered` is a `Source`, so
it nests. A program that wants a fixed list passes only `List(...)`.

## Choosing and launching

A `Resolver` joins a source and the person's `Preferences` and answers "what
happens when they press Open (or Edit) and have chosen nothing":

```go
r := ntopenwith.NewResolver(src, prefs) // prefs from LoadPreferences, or empty
t := ntopenwith.Target{Path: path, Text: isText}

choice, err := r.Resolve(ctx, t, ntopenwith.Edit)
switch {
case errors.Is(err, ntopenwith.ErrChoose):
	apps, _ := r.Choices(ctx, t, ntopenwith.Edit) // show a chooser
case err != nil: // ErrNoEditor, or the source failed
default:
	cmd, err := r.Launch(ctx, t, ntopenwith.Edit, choice.App)
	// cmd is nil for a GUI app, already running; for a terminal app it is
	// unstarted: run it with your terminal attached (tea.ExecProcess, or
	// cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr; cmd.Run()).
}
```

`Choices` is the list for a chooser: hidden apps removed, terminal editors last,
and the effective default marked. `SetDefault` and `SetHidden` change the
preferences, `Preferences()` returns them to save, and `AllApps` lists every app
for a settings screen. `choice.Fallback` is true when a saved default was skipped
because the app is hidden or gone, so you can say so.

| Verb | Chosen, first match wins |
| --- | --- |
| Open | the saved default for the file's kind; else the source's default (the OS's); else the first app; else an error naming the file |
| Edit | the saved default; else, for a `Text` target, the terminal editor (`$VISUAL`/`$EDITOR`) when set; else the source's default editor; else `ErrChoose` (show the Edit chooser), or `ErrNoEditor` when nothing at all can edit it |

A saved default whose app is hidden or no longer offered is skipped, never an
error. What the platforms mean by an Edit default:

| | Open default | Edit default | Edit offers |
| --- | --- | --- | --- |
| macOS | Launch Services' default viewer or editor | its default editor role | apps with the editor role |
| Linux, BSDs | the `[Default Applications]` of `mimeapps.list`; with none, `system` (`xdg-open` decides) | none: `ErrChoose`, so the program shows a chooser | entries categorized TextEditor, Graphics, Office, Development or 3DGraphics |
| Windows | the extension's `UserChoice` | the `edit` verb of its ProgID | the ProgIDs with an `edit` verb |

Terminal editors are never offered for Open: a program that wants one in its
Open menu adds the row itself.

### Preferences

`Preferences` is a plain value, app IDs only: `Defaults` (verb, then kind, to ID)
and `Hidden`. The kind is `Target.Kind`, your own name for the type (`pdf`,
`image/*`), else the extension. `LoadPreferences(path)` and `Save(path)` keep it
in a file of at most 64 KiB, one JSON object with unknown fields and trailing data
rejected, written to a temporary file and renamed, in a folder made `0700`.
`LoadLegacyPreferences` reads gloss's earlier format. A missing file is empty
preferences.

```json
{"version":1,"defaults":{"open":{"pdf":"mac:com.apple.Preview"},"edit":{"image/*":"mac:com.pixelmatorteam.pixelmator.x"}},"hidden":["mac:com.apple.QuickTimePlayerX"]}
```

App IDs are `system`, `terminal-editor`, `mac:<bundle id>`, `xdg:<desktop file id>`,
`win:<ProgID>`, `win:app:<exe name>`, and your own for a `List`.

## Safety

The library never invokes a shell. What it can run is one of four things: the OS's
registered handler, started by the OS's own launcher (`open -a`,
`ShellExecuteExW`); an XDG desktop entry's `Exec`, expanded as the Desktop Entry
specification says; a command compiled into your program with `List`; or
`$VISUAL`/`$EDITOR`. (An installed desktop entry's `Exec` can itself name a
shell, such as `sh -c`; that is the entry's own choice, and field-code rules
cannot make it safe.)

- **Preferences hold IDs only.** An ID launches only if the source offers it *now*
  for that target and verb. `Layered` launches only what it offers now, so an ID
  that is hidden, gone or made up is refused with `ErrNotOffered`.
- **The path** is absolute, cleaned, free of NUL, a regular file (a folder only
  for `Reveal`), and passed as one argument. `{}` must be a whole argument.
- **Edit never uses `$VISUAL` or `$EDITOR` for a target that is not `Text`**, whatever the source says:
  a PNG or a spreadsheet is not handed to vim.
- **XDG `Exec` lines** are refused, not guessed at, when a file code (`%f %F %u
  %U`) sits inside quotes, when there is more than one file code, or when a field
  code is the program itself.
- **Windows** offers only `.exe` programs from `OpenWithList`. A ProgID's own
  command is run by the shell through `ShellExecuteExW`, never rebuilt by the
  library.
- **GUI children** are started with no input and their output discarded, and
  reaped in the background.
- **Names are data.** Bundle names, `Name=` entries and registry strings come
  back unescaped. A terminal program must make them safe before drawing them
  (`ntopen` replaces control characters).
- **"System default" runs the OS's own opener.** For an executable file type
  (`.exe`, `.bat`, `.js`, `.lnk`, a script or an application) that means *running
  the file*. The library cannot tell which files that is for, so a program that
  opens files it did not make should filter such types before offering Open or
  `system`. For a `Directory` target on macOS, `system` opens the folder as
  Finder does: a bundle such as `Foo.app` is launched, and a `.prefPane`
  installed. `Reveal` only selects a folder, so it never runs one.
- No environment variable other than `VISUAL`/`EDITOR` can add a command. `PATH`,
  `HOME` (`USERPROFILE` on Windows), `PATHEXT` and the `XDG_*` variables shape
  the OS's own list, as they do for any file manager.

`ntopenwith.Reveal(path)` shows a file in Finder, Explorer, or (on XDG) its folder
in the default file manager.

## `ntopen`

[`examples/ntopen`](examples/ntopen/main.go) is a small command for trying a
machine; the code is also the shortest complete use of the library:

```sh
ntopen [list|open|edit] FILE [--app ID] [--json] [--text]
```

`list` (the default) prints the Open and Edit choices, the one that would be used
marked `*`, or with `--json` an array of `{verb, id, name, default, terminal}`.
`open` and `edit` launch the default or `--app ID`; a terminal editor runs
attached to your terminal. `--text` says the file is text, which is otherwise
guessed from its start. Exit status: 0 done, 1 failed, 2 no default editor (the
choices are printed, run again with `--app`).

```text
$ ntopen list notes.txt
open:
  * mac:com.apple.TextEdit  TextEdit
    system  System default
    mac:com.microsoft.VSCode  Code

edit:
  * mac:com.apple.TextEdit  TextEdit
    mac:com.microsoft.VSCode  Code
    vim  Vim  (terminal)
```

## Developing

See [DEVELOP.md](DEVELOP.md). `task ci` is the gate.

## License

Released under the [MIT License](./LICENSE.txt).  Part of the Nimble Terminal platform.

Copyright (c) 2026 [Neomantra Corp](https://www.neomantra.com).

----
Made with :heart: and :fire: by the team behind [Nimble Terminal](https://nimbleterminal.com).
