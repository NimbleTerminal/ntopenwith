# ntopenwith: open and edit files with the apps the system knows

Status: design, 2026-10-20. Not yet planned or built.

Module `nimbleterminal.dev/ntopenwith`, source at
`github.com/NimbleTerminal/ntopenwith` (the repo exists, with no commits).

## Why

gloss can hand the file it shows to another program: `w` opens it with the
default app, `W` chooses one, and a terminal editor can take over the
terminal. All of that lives in gloss's `internal/openwith` (about 1,000
lines) with its own compiled-in list of a dozen apps. It ignores what the
operating system already knows: which apps open a PNG, which one is the
default, which ones *edit* it rather than view it.

Other NimbleTerminal programs (go-thinkt, where the idea came from; the
pickers and viewers) want the same thing. The NimbleTerminal habit is to move
reusable code out of gloss into small libraries (ntfilepicker, ntpdf's
`pdfdoc`, nttypeset); this is the next one.

The new capability is **edit with**: a verb of its own beside open, with its
own defaults, so "edit this image" goes to Pixelmator while "open this image"
goes to Preview.

## Goals

- Answer, for a file and a verb (Open or Edit): which apps can do it, and
  which one should by default.
- Prefer the operating system's answer: its registered handlers and its
  defaults, including its default *editor* where it has one.
- Let a developer supply their own list instead, or on top, without the
  library reading the system at all.
- Launch the chosen app safely: no shell, no command line built from a
  preference file.
- Keep the user's choices (a default per kind and verb, hidden apps) in a
  small value the program can store, and a bounded file format for those
  that want one.
- Let gloss move onto it with no loss: its keys, saved preferences, terminal
  editor and safety rules.

## Non-goals

- No user interface. The chooser, the manage-apps list and any preferences
  editor are the program's; gloss keeps its own. (Evan: "the UX could be
  opinionated.")
- No Bubble Tea dependency. Running a terminal editor attached is the
  program's job (gloss uses `tea.ExecProcess`); the library gives it the
  `*exec.Cmd`.
- No user-defined commands from configuration. A developer's list is Go code
  compiled into their program.
- No URL handlers, no "open with" for URLs, no file associations written
  back to the OS ("make this the system default").
- No browser support beyond building: under `js/wasm` the system source is
  empty and launching returns `ErrUnsupported`.

## Anti-goals

It fails, even if it works, if:

- a preference file, an environment variable other than `VISUAL`/`EDITOR`,
  or a file name can make it run a program the user did not pick from the
  OS's own list or the developer's compiled list;
- it needs CGO, or a helper process on every query;
- a developer who wants a fixed list must still let it read the system;
- Edit on a binary file (an image, a PDF, a spreadsheet) ever hands the
  file to `$EDITOR`.

(The OS's own list includes what the platform's standards say shapes it:
`PATH` for finding programs, and on XDG systems `XDG_DATA_HOME`,
`XDG_DATA_DIRS`, `XDG_CONFIG_HOME`, `XDG_CONFIG_DIRS` and
`XDG_CURRENT_DESKTOP`, as every file manager reads them. *Builder's-check
call.*)

## Concepts

**Target**: the file, or only its type. `Path` (absolute after validation;
may be empty for a question about a type, as `--apps` and a settings screen
ask), `Ext` (the type as an extension, `.png`, used when there is no Path and
as the type otherwise when the OS cannot tell), `Kind` (optional,
the program's own name for the file's type, such as gloss's `pdf`, `image/*`,
`markdown`), `Directory` (only Reveal applies), `Text` (the original is text,
so a terminal editor makes sense; the program decides, gloss from its format
list).

**Verb**: `Open` or `Edit`. Reveal (show in Finder / Explorer / the file
manager) is a separate function, not a verb with apps.

**App**: what a source offers. `ID` (stable, namespaced by source, see
below), `Name` (for display), `Verbs` it is offered for, `Terminal` (runs
attached in the terminal), `Default` per verb when the source says so. An App
is data; how it launches is private to the source that produced it.

**Source**: the interface every list of apps implements.

```go
type Source interface {
    // Apps are those that can act on t with v, the source's default first
    // when it has one.
    Apps(ctx context.Context, t Target, v Verb) ([]App, error)
    // Launch starts app on t. A GUI app is started and left running; a
    // terminal app is returned as a command for the caller to run attached.
    Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error)
}
```

The returned `*exec.Cmd` is non-nil only for `Terminal` apps; the library
never starts those itself.

## Sources the library provides

### `System()`: the operating system's index

Chosen at build time per OS. Each reports apps for the target's file (by
path, so the OS uses its own type detection) with its default first; a
Target with no Path is answered from its `Ext` (macOS: the extension's UTI
with `LSCopyAllRoleHandlersForContentType` and
`LSCopyDefaultRoleHandlerForContentType`; XDG: the MIME type from the globs;
Windows: the extension's keys). Launching needs a Path.

Every `System()` also offers the app **`system`** ("System default") for
Open: it hands the file to the OS's own opener (`open`, `xdg-open`,
`ShellExecuteEx` with `open` and no class), so it works even when the index
lists nothing. It is the source's Open default when the index names none.
(*Builder's-check call*, keeping gloss's row and `d` behavior.)

The C calls named here are deprecated since macOS 12, and their NSWorkspace
replacements take no role, so Open and Edit could not be told apart. Use
them, behind one small interface so they can be replaced. (*Builder's-check
call.*)

- **macOS: Launch Services**, called through `github.com/ebitengine/purego`
  (no CGO; Core Foundation and Core Services are plain C APIs):
  `LSCopyApplicationURLsForURL(url, role)` for the lists and
  `LSCopyDefaultApplicationURLForURL(url, role, nil)` for the default, with
  `kLSRolesViewer|kLSRolesEditor` for Open and `kLSRolesEditor` for Edit.
  App ID `mac:<bundle identifier>`, name from the bundle's display name.
  Launch: `open -a <bundle path> <file>`. A spike with JXA on 2026-10-20 got
  default, editors and viewers for four files in 65 ms total; real data
  is noisy (QuickTime "edits" PNG; Bambu Studio only "views" STL).
- **Linux and the BSDs: XDG**, pure Go: the MIME type from
  `shared-mime-info` globs (extension, then magic is not required in v1; fall
  back to `application/octet-stream`), handlers from `mimeinfo.cache` and the
  `MimeType=` of `.desktop` files in `$XDG_DATA_HOME` and `$XDG_DATA_DIRS`,
  defaults from `mimeapps.list` in the spec's order (desktop-specific first),
  honoring `[Removed Associations]`. Hidden, `NoDisplay`, `TryExec`-missing
  and `OnlyShowIn`/`NotShowIn` entries are dropped. App ID
  `xdg:<desktop file id>`. XDG has no editor role: Edit lists entries whose
  `Categories` include `TextEditor`, `Graphics`, `Office`, `Development` or
  `3DGraphics`, and has **no default** (see "Choosing an app").
  Launch: the entry's `Exec` expanded per the Desktop Entry specification
  (`%f %F %u %U` with the one path, `%i %c %k` as specified, other field
  codes dropped), split by the spec's quoting rules, run with
  `exec.Command`, never a shell. `Terminal=true` entries are terminal apps.
  Entries with `DBusActivatable=true` and no usable `Exec` are skipped.
- **Windows: the registry and the shell**, pure Go through
  `golang.org/x/sys/windows` and `windows/registry`: the extension's
  `UserChoice` ProgID for the Open default, `OpenWithProgids` and
  `OpenWithList` (user and machine) for the list, the ProgID's
  `shell\edit` verb for Edit and its default. Names from
  `FriendlyAppName`/`AssocQueryString(ASSOCSTR_FRIENDLYAPPNAME)`. App ID
  `win:<ProgID>` or `win:app:<exe name>` for `OpenWithList` entries.
  Launch: `ShellExecuteEx` with the verb (`open` or `edit`) and, for a
  specific app, `SEE_MASK_CLASSNAME` with its ProgID, so Windows runs its own
  registered command.

All three are read on each `Apps` call with a short cache (keyed by kind or
extension, a few seconds) that a program can drop with `Refresh()`. No
query starts a process on macOS or Windows; on Linux it reads files.

### `List(apps ...ListApp)`: a developer's own

A fixed list that never reads the system. Each entry:

```go
type ListApp struct {
    ID, Name string
    Verbs    []Verb   // Open, Edit or both.
    Kinds    []string // Target kinds it accepts; empty accepts all.
    Text     bool     // Only for Text targets (terminal editors).
    Command  []string // argv; "{}" is replaced by the path, else it is appended.
    Terminal bool     // Run attached by the caller.
    Bundle   string   // macOS: launch with open -a this bundle instead of Command.
    Find     []string // Executables or bundles whose presence makes it available.
    Default  []Verb   // Verbs it is this list's default for.
}
```

Availability is checked when the list is built (`exec.LookPath`, or the
bundle in `/Applications`, `~/Applications`, `/System/Applications`), and
again before launch. `Command` is argv from Go code: the substitution rules
are gloss's `BuildCommand` (whole-argument `{}`, absolute paths, no NUL).

Also provided, for building lists:

- `TerminalEditor()`: `$VISUAL`, else `$EDITOR`, parsed as gloss does today
  (an exact executable path, or POSIX words via shlex without expansion; at
  most 8 KiB and 64 words), `Text` and `Terminal`, available when set. Read
  at launch, so a change in the environment is seen.
- `TerminalEditors()`: nvim, vim, hx, nano, micro, emacs `-nw`, each
  available when on `PATH`, `Text` and `Terminal`, offered for Edit.

Terminal editors are offered for Edit only. A program that wants one in its
Open chooser too (gloss's "Edit in terminal" row) asks for the Edit choices
and adds the row itself.
- `Reveal(path)`: Finder (`open -R`), Explorer (`/select,`), or the
  containing folder with the XDG default file manager.

### `Layered(sources ...Source)`: one on top of another

Asks each source, merges by app, and keeps the first source's default
unless a later one is told to override (`Override(src)`). A later source
corrects an earlier one:

- `Hide(ids ...string)`: drop apps (QuickTime from Edit for images).
- `Rename(id, name)`, `Rank(ids ...string)`: names and order.
- A `ListApp` whose ID names a system app (`mac:com.microsoft.VSCode`)
  replaces that app's name or verbs instead of adding a second row.
- `Alias(id, systemID)`: keep an old ID (gloss's `vscode`) meaning the system
  app, so saved preferences survive.

`Layered` is itself a Source, so it nests. A developer who wants no system
reads passes only `List(...)`; one who wants only the system passes
`System()`; gloss passes `Layered(System(), glossCorrections, List(TerminalEditor(), TerminalEditors()...))`.

## Choosing an app

`Resolver` combines a Source and Preferences and answers "what happens when
the user presses Open (or Edit) with nothing chosen":

- **Open**: the saved default for the target's kind and Open, if still
  available and not hidden; else the source's default; else the first app;
  else an error that names the file.
- **Edit**:
  1. the saved default for the kind and Edit;
  2. for a `Text` target, `$VISUAL`/`$EDITOR` when set;
  3. the source's default editor (macOS role, Windows `edit` verb);
  4. otherwise `ErrChoose`: the program should show its Edit chooser.
     For a binary target with no editor at all, `ErrNoEditor` naming the
     file, never a fallback to a viewer or to `$EDITOR`.

A saved default that is gone (uninstalled, hidden) falls through, and the
result says so (`Fallback: true`) so the program can tell the user, as gloss
does today ("saved app unavailable").

`Resolver.Choices(t, v)` is the list a chooser shows: hidden apps removed,
the effective default marked, terminal editors last.

`Resolver.AllApps(kinds)` is every app any source offers for those kinds and
either verb (asked by `Ext`, the program giving one extension per kind), for
a settings screen that hides apps, as gloss's `m` list does.

Setting a default: `d` on an app stores its ID for the kind and verb; on
`system` (Open) it removes the saved default, so the OS decides again. The
Resolver refuses an ID the source does not offer for that kind and verb.

## Preferences

```go
type Preferences struct {
    Defaults map[Verb]map[string]string // verb → kind → app ID
    Hidden   []string                   // app IDs
}
```

A plain value: the program may store it anywhere. For those who want a file,
`LoadPreferences(path)` and `(p Preferences) Save(path)` keep gloss's rules:
at most 64 KiB, one JSON object, unknown fields rejected, trailing data
rejected, written to a temporary file in the same folder and renamed, folder
created `0700`. Loading drops entries whose kind or ID is malformed; it does
not check installation (that is the Resolver's job at use, so an app
reinstalled later works again).

Kinds are the program's: gloss's `pdf`, `image/*`. When a target has no
Kind, the key is its extension, lower case (`.png`). (*Builder's-check
call*: no new return value on Source just for keys.)

Format version 1:

```json
{"version":1,"defaults":{"open":{"pdf":"mac:com.apple.Preview"},"edit":{"image/*":"mac:com.pixelmatorteam.pixelmator.x"}},"hidden":["mac:com.apple.QuickTimePlayerX"]}
```

Programs give the new format its own file name. gloss writes
`open-with-v1.json` and, when that is missing, reads its old
`open-with.json` (`{"defaults":{kind:id},"hidden":[...]}`) with
`LoadLegacyPreferences` as Open defaults, never writing to it, so an older
gloss on the same machine keeps working with its own file. gloss's old IDs
resolve through `Alias`. (*Builder's-check call.*)

## Safety

- What runs is either the OS's registered handler, launched by the OS's own
  mechanism (`open -a`, `ShellExecuteEx`), an XDG entry's `Exec` expanded
  without a shell, a `List` command compiled into the program, or
  `$VISUAL`/`$EDITOR`.
- Preferences hold IDs only, and an ID launches only if the source still
  offers it for that target and verb.
- The path is absolute, cleaned, has no NUL, is a regular file (or a
  directory for Reveal), and is passed as one argument; a path beginning with
  `-` is passed as `./-…` or after `--` where the launcher allows.
- GUI children are started and reaped in the background; no stdin, and
  their output discarded.
- Names from the system (bundle names, desktop `Name=`, registry strings) are
  data: the library returns them unescaped, and documents that a terminal
  program must escape them for display (gloss already does).

## Package layout

- `ntopenwith` (root): Target, Verb, App, Source, Resolver, Preferences,
  List, Layered, Reveal, TerminalEditor(s), errors.
- `ntopenwith/system`: `System()` with `system_darwin.go`,
  `system_xdg.go` (linux, freebsd, openbsd, netbsd), `system_windows.go`,
  `system_other.go` (empty source, including js/wasm).
- `ntopenwith/internal/xdg`, `internal/launchservices`, `internal/winassoc`:
  the readers, each testable without the OS (fixture directories for XDG;
  an interface over the C calls for macOS; an interface over the registry
  for Windows).
- `examples/ntopen`: a small command, `ntopen [list|open|edit] FILE
  [--app ID] [--json]`, for trying a machine and for the README.

Root `go.mod` requires only `purego`, `x/sys` and `shlex`.

## Testing

- Unit tests with fakes for each reader; XDG with fixture trees
  (`mimeapps.list` precedence, removed associations, `Exec` field codes and
  quoting, `TryExec`, `Terminal=true`).
- Resolver tables for every row of "Choosing an app", including the binary
  file with only `$EDITOR` set (must not use it).
- Preferences: bounds, malformed input, gloss's old format, atomic write.
- CI on macOS, Ubuntu and Windows runners runs the system source against the
  real OS and asserts only what every runner has (a `.txt` has some Open
  handler; queries return without error).
- A launch test per OS with a `List` app that is a test helper binary
  recording its arguments: the path arrives as one argument, unchanged.

## Moving gloss onto it

gloss keeps its keys and screens (`w`, `W`, `d`, `m`, Alt+Enter, `--apps`)
and its editor handoff, and replaces `internal/openwith` with:

- its kinds and `Text` rule (today's `Editable`), on the Target;
- `Layered(system.System(), its corrections, List(TerminalEditor(),
  TerminalEditors()...))`, with `Alias` for `vscode`, `cursor`, `zed`,
  `sublime`, `preview`, `skim`;
- `LoadPreferences` on the existing `gloss/open-with.json`;
- an Edit binding and Edit chooser (keys are gloss's decision, in its own
  plan), and `--apps` growing a `verb` field.

`--apps --json` and the schema keep their current fields; new ones are
additions.

## Left to the builder

- purego details on macOS (the CF calls, release of returned objects), and
  whether a missing bundle display name falls back to the file name.
- The exact XDG category list for Edit, and the cache lifetime.
- Whether Windows lists `OpenWithList` apps that have no ProgID.
- README, AGENTS.md, DEVELOP.md, CHANGELOG, Taskfile and CI in the
  NimbleTerminal shape (as ntfilepicker), and a v0.1.0 tag once gloss has
  adopted it.
