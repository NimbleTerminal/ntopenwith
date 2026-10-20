# ntopenwith Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go library that lists the apps able to open or edit a file, picks one (the OS's default first, the user's saved choice over it), and launches it safely, with the OS index or a developer's own list as interchangeable sources.

**Architecture:** The root package holds the value types, the `Source` interface, the developer's `List`, `Layered` composition, terminal editors, preferences and the `Resolver`. Package `system` provides `System()`, one file per OS, over three internal readers (`internal/xdg`, `internal/launchservices`, `internal/winassoc`) that are tested with fakes or fixtures. No UI.

**Tech Stack:** Go 1.26.8+, `github.com/ebitengine/purego` (macOS, no CGO), `golang.org/x/sys/windows` and `windows/registry`, `github.com/google/shlex`. Task for builds, GitHub Actions on macOS, Ubuntu and Windows.

**Spec:** `docs/superpowers/specs/2026-10-20-ntopenwith-design.md`

## Global Constraints

- Module `nimbleterminal.dev/ntopenwith`; `go 1.26.8` in `go.mod`; root requires only purego, x/sys and shlex (stdlib otherwise). `CGO_ENABLED=0` must build on every OS.
- No UI, no Bubble Tea. Terminal apps are returned as an unstarted `*exec.Cmd`; the library never starts them.
- Never a shell. What runs: an OS handler via the OS's launcher (`open -a`, `ShellExecuteExW`), an XDG `Exec` expanded per the Desktop Entry spec, a `List` command compiled into the program, or `$VISUAL`/`$EDITOR`.
- Preferences hold IDs only; an ID launches only if the source still offers it for that target and verb.
- Paths launched are absolute, cleaned, NUL-free, a regular file (a directory only for `Reveal`), and passed as one argument.
- Edit on a non-`Text` target never uses `$VISUAL`/`$EDITOR`.
- Preference files: at most 64 KiB, one JSON object, unknown fields and trailing data rejected, written to a temp file in the same folder and renamed, folder created `0700`.
- App IDs: `system`, `terminal-editor`, `mac:<bundle id>`, `xdg:<desktop file id>`, `win:<ProgID>`, `win:app:<exe name>`; `List` IDs are the developer's.
- Under `js/wasm` everything builds; `System()` lists nothing and every launch returns `ErrUnsupported`.
- Copyright header style and repo shape follow `../ntfilepicker` (Taskfile with `GOWORK: 'off'`, `-mod=readonly`, `task ci`, AGENTS.md, DEVELOP.md, CHANGELOG.md, LICENSE.txt, CODE_OF_CONDUCT.md).

## Review Focus

1. A path with spaces, quotes, `$`, a leading `-` in its base name, or non-ASCII letters must reach the launched program as exactly one unchanged argument. Test: Task 2 `TestLaunchPassesPathAsOneArgument`.
2. A saved default whose app was uninstalled or hidden falls through with `Fallback: true`; it does not fail. Test: Task 6 `TestResolveSavedAppGone`.
3. Extension case and absence: `PHOTO.PNG`, `.png`, and a file with no extension give stable keys and lookups. Tests: Task 1 `TestTargetKey`, Task 8 `TestTypeOfCaseAndMissing`.
4. A corrupt, oversized or wrong-shaped preference file is an error from `LoadPreferences`, and a program can still resolve with empty preferences. Test: Task 5 `TestLoadRejects`.
5. Edit on a binary file with `$EDITOR` set and nothing else installed returns `ErrNoEditor`, never the terminal editor. Test: Task 6 `TestEditBinaryNeverUsesEditor`.

---

### Task 1: Module scaffold and core types

**Files:**
- Create: `go.mod`, `LICENSE.txt` (MIT, copy `../ntfilepicker/LICENSE.txt`), `CODE_OF_CONDUCT.md` (copy), `Taskfile.yml`, `.github/workflows/ci.yml`, `.gitignore` (`bin/`, `.task/`)
- Create: `doc.go`, `target.go`, `source.go`
- Test: `target_test.go`

**Interfaces:**
- Produces:
  - `type Verb string`; `const Open Verb = "open"`, `Edit Verb = "edit"`.
  - `type Target struct { Path, Ext, Kind string; Directory, Text bool }`
  - `func (t Target) Type() string`: lower-case extension with dot, from `Ext`, else `filepath.Ext(Path)`; `""` when neither has one.
  - `func (t Target) Key() string`: `Kind` if set, else `Type()`.
  - `func CheckPath(path string, directory bool) (string, error)`: absolute, `filepath.Clean`, rejects `""` and NUL, `os.Stat`; regular file unless `directory`, then a directory.
  - `type App struct { ID, Name string; Terminal, Default bool; from Source }` (`from` unexported, set by `Layered`).
  - `type Source interface { Apps(ctx context.Context, t Target, v Verb) ([]App, error); Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error) }`
  - `const SystemID = "system"`, `TerminalEditorID = "terminal-editor"`
  - `var ErrUnsupported, ErrChoose, ErrNoEditor, ErrNotOffered error` (`errors.New`, messages: "not supported on this platform", "no default app: choose one", "no app can edit this file", "app is not offered for this file").

- [ ] **Step 1: Write the failing tests** in `target_test.go`

```go
func TestTargetKey(t *testing.T) {
	cases := []struct{ t Target; key, typ string }{
		{Target{Path: "/a/PHOTO.PNG"}, ".png", ".png"},
		{Target{Ext: ".Png"}, ".png", ".png"},
		{Target{Path: "/a/README"}, "", ""},
		{Target{Path: "/a/x.pdf", Kind: "pdf"}, "pdf", ".pdf"},
		{Target{Path: "/a/x.tar.gz"}, ".gz", ".gz"},
	}
	// assert Key() and Type() for each
}
func TestCheckPath(t *testing.T) {
	// relative path becomes absolute; "a/../b.txt" cleans;
	// "" and "x\x00y" error; a directory errors unless directory=true;
	// a regular file with directory=true errors; a missing file errors.
}
```

- [ ] **Step 2: Run** `GOWORK=off go test ./...` — expect build failure (undefined `Target`).
- [ ] **Step 3: Implement** `doc.go` (package comment: what it is, the three sources, that there is no UI), `target.go`, `source.go` with the signatures above. `go.mod`: `module nimbleterminal.dev/ntopenwith`, `go 1.26.8`.
- [ ] **Step 4: Add `Taskfile.yml`** modeled on ntfilepicker's: `test`, `test-race`, `vet`, `fmt-check` (`gofmt -l .` empty), `go-tidy-check` (`go mod tidy -diff`), `cross` (`GOOS=windows go vet ./...`, `GOOS=linux go vet ./...`, `GOOS=darwin go vet ./...`, `GOOS=freebsd go build ./...`, `GOOS=js GOARCH=wasm go build ./...`, each with `CGO_ENABLED=0`), `staticcheck` (`go run honnef.co/go/tools/cmd/staticcheck@latest ./...` as ntfilepicker does), `build` (`go build -o bin/ntopen ./examples/ntopen`, added in Task 12), and `ci` running all but `build`. `ci.yml`: a matrix `os: [ubuntu-latest, macos-latest, windows-latest]`, Go `1.27.2`, `go-task/setup-task@v2`, `task ci`.
- [ ] **Step 5: Run** `task ci` — PASS on this Mac.
- [ ] **Step 6: Commit** `git add -A && git commit -m "feat: module, targets and the Source interface"`

### Task 2: Commands and detached launching

**Files:**
- Create: `command.go`
- Test: `command_test.go`

**Interfaces:**
- Consumes: `CheckPath`.
- Produces:
  - `func Command(argv []string, path string) ([]string, error)`: gloss's `BuildCommand` rules (port `../gloss/internal/openwith/launch.go`): `argv[0]` non-empty and without `{}`; an argument exactly `{}` becomes `path`; `{}` inside a longer argument is an error; with no `{}`, `path` is appended; `path` must be absolute and NUL-free.
  - `func Start(argv []string) error`: `exec.Command`, `Stdin`/`Stdout`/`Stderr` nil (discarded), `Start`, reap in a goroutine.
  - `var startCommand = func(c *exec.Cmd) error { ... }` test hook used by `Start`.

- [ ] **Step 1: Write the failing tests**

```go
func TestCommand(t *testing.T) {
	// {"open","-a","/Applications/X.app"} + "/p/a b.png" → {"open","-a","/Applications/X.app","/p/a b.png"}
	// {"code","--wait","{}","--new"} → path replaces "{}" in place
	// {"x","a{}b"} → error; {"","x"} → error; {"{}"} → error
	// relative path → error; "/p/a\x00b" → error
}
// TestLaunchPassesPathAsOneArgument builds this test binary as a helper:
// when GO_NTOPENWITH_HELPER=1, TestHelperRecordsArgs writes os.Args[1:] as
// JSON to $GO_NTOPENWITH_OUT and exits. The test runs Start with argv
// {os.Args[0], "-test.run=^TestHelperRecordsArgs$", "--", "{}"} for each of
// these base names, each a real file in t.TempDir():
//   "a b.png", `q"uote's.txt`, "$HOME.txt", "-dash.txt", "naïve 文件.md"
// and asserts the recorded args end with exactly the absolute path, once.
```

- [ ] **Step 2: Run** `go test -run 'TestCommand|TestLaunchPasses' .` — FAIL (undefined).
- [ ] **Step 3: Implement** `command.go`. `Start` must not wait for the app; the test waits for the output file to appear (poll up to 5 s).
- [ ] **Step 4: Run** the tests — PASS. Run `task cross` — PASS.
- [ ] **Step 5: Commit** `feat: build argv without a shell and start apps detached`

### Task 3: Terminal editors

**Files:**
- Create: `editor.go`
- Test: `editor_test.go` (port the cases of `../gloss/internal/openwith/editor_test.go`)

**Interfaces:**
- Consumes: nothing beyond Task 1.
- Produces (used by Task 4's `ListApp`):
  - `func editorArgs(setting string) ([]string, error)`: port of gloss's `editorArguments` (exact executable path first via `exec.LookPath`, else `shlex.Split`; ≤ 8192 bytes, no NUL, 1–64 words; empty setting → error "set VISUAL or EDITOR to a terminal editor, for example EDITOR=nano").
  - `func editorSetting() string`: trimmed `$VISUAL`, else trimmed `$EDITOR`.

- [ ] **Step 1: Write the failing tests**: `TestEditorArgs` (exact path with spaces kept whole; `nvim -p` → two words; `"emacs -nw"` quoted; empty → error naming VISUAL; 8193 bytes → error; 65 words → error; NUL → error) and `TestEditorSetting` (`VISUAL` wins over `EDITOR`; whitespace-only `VISUAL` falls to `EDITOR`) using `t.Setenv`.
- [ ] **Step 2: Run** `go test -run 'TestEditor' .` — FAIL.
- [ ] **Step 3: Implement** `editor.go`.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: parse VISUAL and EDITOR as gloss does`

### Task 4: List, terminal editor apps, and Layered

**Files:**
- Create: `list.go`, `layered.go`
- Test: `list_test.go`, `layered_test.go`

**Interfaces:**
- Consumes: `Command`, `Start`, `editorArgs`, `editorSetting`, `CheckPath`.
- Produces:
  - `type ListApp struct { ID, Name string; Verbs []Verb; Kinds []string; Text bool; Command []string; Terminal bool; Bundle string; Find []string; Default []Verb; command func() ([]string, error) }`
  - `func List(apps ...ListApp) Source`. An app is offered for `(t, v)` when `v` is in `Verbs`; `Kinds` is empty or contains `t.Kind` or `t.Type()`; not `Text`, or `t.Text`; and it is available. Available: each name in `Find` (default: `Command[0]`, or `Bundle`) resolves with `lookPath` or, for a name ending `.app`, exists under `bundleDirs()` (`$HOME/Applications`, `/Applications`, `/System/Applications`). `Default: true` on the returned App when `v` is in `Default`; defaults first. Launch re-checks availability; with `Bundle` (darwin) runs `Start({"open","-a",<found bundle path>}+path)`; with `Terminal` returns `exec.Command(Command(argv, path)...)` unstarted; else `Start(Command(argv, path))`. An app not offered → `ErrNotOffered`.
  - `var lookPath = exec.LookPath`, `var bundleDirs func() []string` (test hooks).
  - `func TerminalEditor() ListApp`: `ID: TerminalEditorID`, `Name: "Edit in terminal"`, `Verbs: {Edit}`, `Text`, `Terminal`, available when `editorArgs(editorSetting())` succeeds, `command` reads the environment at launch.
  - `func TerminalEditors() []ListApp`: IDs `nvim`, `vim`, `hx`, `nano`, `micro`, `emacs` (Command `{"emacs","-nw"}`), Names `Neovim`, `Vim`, `Helix`, `nano`, `micro`, `Emacs`; Verbs `{Edit}`, `Text`, `Terminal`.
  - `func Layered(sources ...Source) Source`, `func Hide(ids ...string) Source`, `func Rename(id, name string) Source`, `func Rank(ids ...string) Source`, `func Alias(id, target string) Source`, `func Override(s Source) Source`.
  - `type Canonicalizer interface { Canonical(id string) string }`, implemented by the `Layered` value: maps an `Alias` id to its target, else returns `id`.

  `Layered` semantics (the algorithm the tests pin): ask each non-correction source in order; append apps with new IDs, recording `from`; an app whose ID already exists only replaces `Name`. The default is the first source's default app, unless an `Override` source offers one. Then apply the corrections in the order given: `Hide` removes, `Rename` renames, `Alias(id, target)` adds nothing but feeds `Canonical`. Order the result: the default first, then `Rank` IDs in the given order, then the rest in source order, terminal apps last. `Launch` sends to `app.from` (or to the first source offering `app.ID` when `from` is nil), resolving an alias first. An error from one source is returned only if no source answered.

- [ ] **Step 1: Write the failing tests**

```go
func TestListOffers(t *testing.T)            // Kinds by Kind and by ".ext"; Text-only apps hidden for binary targets; Find missing → not offered; Default verbs put the app first with Default=true
func TestListLaunchTerminalReturnsCmd(t *testing.T) // Terminal app → non-nil *exec.Cmd, not started (cmd.Process == nil), args end with the path
func TestTerminalEditorReadsEnvAtLaunch(t *testing.T) // EDITOR set after List() is built is still used; unset → not offered
func TestLayeredMergesAndCorrects(t *testing.T)
	// src A (fake): apps a1(default), q; src B: List naming "q" with Name "Q!" and a new app b1
	// Layered(A, B, Hide("q"), Rename("b1","Bee"), Rank("b1")) → [a1(default), Bee]
func TestLayeredOverrideDefault(t *testing.T) // Override(B) with a default beats A's default
func TestLayeredLaunchRoutesToSource(t *testing.T) // fake sources record which got Launch
func TestAliasCanonical(t *testing.T)        // Alias("vscode","mac:com.microsoft.VSCode"): Canonical("vscode") == "mac:com.microsoft.VSCode"; Launch of an App{ID:"vscode"} reaches the target app
```

- [ ] **Step 2: Run** `go test -run 'TestList|TestTerminalEditor|TestLayered|TestAlias' .` — FAIL.
- [ ] **Step 3: Implement** `list.go` and `layered.go` (corrections are unexported types implementing `Source` with `Apps` returning nil, plus an unexported `apply([]App) []App`).
- [ ] **Step 4: Run** — PASS; `task cross` — PASS.
- [ ] **Step 5: Commit** `feat: developer lists, terminal editors and layered sources`

### Task 5: Preferences

**Files:**
- Create: `preferences.go`
- Test: `preferences_test.go`

**Interfaces:**
- Produces:
  - `type Preferences struct { Version int \`json:"version"\`; Defaults map[Verb]map[string]string \`json:"defaults"\`; Hidden []string \`json:"hidden"\` }`
  - `func LoadPreferences(path string) (Preferences, error)`: missing file → empty, nil. Enforces the 64 KiB / one object / no unknown fields / no trailing data rules; `version` must be 1; verbs other than `open`/`edit` are an error; entries whose key or ID is empty, over 256 bytes, or holds a control character are dropped.
  - `func LoadLegacyPreferences(path string) (Preferences, error)`: gloss's `{"defaults":{kind:id},"hidden":[...]}` (same bounds), returned as `Defaults[Open]`, `Version: 1`; missing file → empty, nil.
  - `func (p Preferences) Save(path string) error`: sets `Version: 1`, marshals, ≤ 64 KiB, `MkdirAll(dir, 0700)`, `CreateTemp` in dir, write, close, `Rename`.
  - `func (p Preferences) Default(v Verb, key string) string`, `func (p *Preferences) SetDefault(v Verb, key, id string)` (empty id deletes), `func (p Preferences) IsHidden(id string) bool`, `func (p *Preferences) SetHidden(id string, hidden bool)` (no duplicates).

- [ ] **Step 1: Write the failing tests**: `TestPreferencesRoundTrip` (save, load equal; file mode of a new dir is 0700 on unix); `TestLoadRejects` (64 KiB + 1, `[]`, `{"version":2}`, unknown field, trailing `{}`, verb `"print"` each → error; and `NewResolver(src, Preferences{})` still resolves afterwards); `TestLoadDropsBadEntries`; `TestLegacy` (gloss's file `{"defaults":{"pdf":"skim"},"hidden":["zed"]}` → `Defaults[Open]["pdf"]=="skim"`, `Hidden==["zed"]`); `TestSaveIsAtomic` (a failing write leaves the old file intact: make the dir read-only after the first save on unix; skip on windows).
- [ ] **Step 2: Run** `go test -run 'TestPreferences|TestLoad|TestLegacy|TestSave' .` — FAIL.
- [ ] **Step 3: Implement** `preferences.go`.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: preferences with a bounded file format and gloss's old one`

### Task 6: Resolver

**Files:**
- Create: `resolver.go`
- Test: `resolver_test.go` (fake sources only)

**Interfaces:**
- Consumes: `Source`, `App`, `Preferences`, `Canonicalizer`, `CheckPath`, the errors and IDs.
- Produces:
  - `func NewResolver(src Source, prefs Preferences) *Resolver`
  - `type Choice struct { App App; Fallback bool }`
  - `func (r *Resolver) Resolve(ctx context.Context, t Target, v Verb) (Choice, error)`: the spec's "Choosing an app", exactly. Open: saved (canonicalized, offered, not hidden) → source default → first app → `ErrChoose` wrapped with the file name when nothing is offered. Edit: saved → for `t.Text`, the offered `terminal-editor` app → the source's default → `ErrChoose` if any app is offered, else `ErrNoEditor`. `Fallback` is true when a saved default existed but was skipped.
  - `func (r *Resolver) Choices(ctx context.Context, t Target, v Verb) ([]App, error)`: hidden removed; `Default` set on the app `Resolve` would pick and cleared elsewhere; terminal apps last.
  - `func (r *Resolver) AllApps(ctx context.Context, types map[string]string) ([]App, error)`: kind → extension; asks `Target{Kind: k, Ext: e}` for both verbs; unique by ID, sorted by `Name`; includes hidden apps (a settings screen shows them to unhide); excludes `system` and `terminal-editor`.
  - `func (r *Resolver) SetDefault(ctx context.Context, t Target, v Verb, id string) error`: `system` (Open) clears; else the id must be in `Choices(t, v)` or `ErrNotOffered`.
  - `func (r *Resolver) SetHidden(id string, hidden bool) error`: `system` and `terminal-editor` → error.
  - `func (r *Resolver) Preferences() Preferences` (a copy).
  - `func (r *Resolver) Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error)`: `CheckPath(t.Path, t.Directory)` then the source's `Launch`.
  - Safe for concurrent use (a mutex around preferences).

- [ ] **Step 1: Write the failing tests**: one table `TestResolve` with a fake source per row covering every rule above, plus:

```go
func TestResolveSavedAppGone(t *testing.T)        // saved "x" not offered → source default, Fallback == true
func TestEditBinaryNeverUsesEditor(t *testing.T) // EDITOR=nano; Layered(List(TerminalEditor())); Target{Path: png, Text: false}; Edit → errors.Is(err, ErrNoEditor)
func TestEditTextUsesEditor(t *testing.T)        // same with Text: true and no saved default → terminal-editor
func TestEditSavedBeatsEditor(t *testing.T)
func TestChoicesHiddenAndOrder(t *testing.T)
func TestSetDefaultRefusesUnoffered(t *testing.T)
func TestSetDefaultSystemClears(t *testing.T)
func TestAllApps(t *testing.T)
```

- [ ] **Step 2: Run** `go test -run 'TestResolve|TestEdit|TestChoices|TestSetDefault|TestAllApps' .` — FAIL.
- [ ] **Step 3: Implement** `resolver.go`.
- [ ] **Step 4: Run** `go test -race ./...` — PASS.
- [ ] **Step 5: Commit** `feat: resolve Open and Edit with saved choices and OS defaults`

### Task 7: Reveal, and the system package's shared shell

**Files:**
- Create: `reveal.go`, `system/system.go`, `system/cache.go`, `system/system_other.go` (`//go:build !darwin && !windows && !linux && !freebsd && !openbsd && !netbsd`, which includes js/wasm)
- Test: `reveal_test.go`, `system/cache_test.go`

**Interfaces:**
- Produces:
  - `func Reveal(path string) error`: `CheckPath(path, <is dir>)`; darwin `Start({"open","-R",p})` (a directory: `{"open",p}`); windows `Start({"explorer.exe","/select,"+p})` (a directory: `{"explorer.exe",p}`); XDG `Start({"xdg-open", dir})` with `dir` the file's folder or the directory; other → `ErrUnsupported`.
  - `package system`: `func System() ntopenwith.Source` (one per process, built lazily), `func Refresh()` (empties the cache).
  - `type cache` keyed by `(type or path's type, verb)` with a 5-second lifetime, mutex-guarded; `func (c *cache) get(key string, v ntopenwith.Verb, fill func() ([]ntopenwith.App, error)) ([]ntopenwith.App, error)`.
  - `system_other.go`: `Apps` returns nil, nil; `Launch` returns `ntopenwith.ErrUnsupported`.

- [ ] **Step 1: Write the failing tests**: `TestRevealArgs` (swap the `startCommand` hook; for a file and a directory assert the argv for `runtime.GOOS`); `TestCacheExpires` (fill called once within the lifetime, again after it, and again after `Refresh`).
- [ ] **Step 2: Run** `go test ./... -run 'TestReveal|TestCache'` — FAIL.
- [ ] **Step 3: Implement.** The cache's clock is a package var for the test.
- [ ] **Step 4: Run** — PASS; `task cross` (js/wasm must build) — PASS.
- [ ] **Step 5: Commit** `feat: Reveal, and the system package's cache`

### Task 8: XDG reader

**Files:**
- Create: `internal/xdg/env.go`, `internal/xdg/mime.go`, `internal/xdg/desktop.go`, `internal/xdg/apps.go`, `internal/xdg/exec.go`
- Test: `internal/xdg/*_test.go` with fixture trees in `internal/xdg/testdata/`

**Interfaces:**
- Produces:
  - `type Env struct { DataHome string; DataDirs []string; ConfigHome string; ConfigDirs []string; Desktops []string }`; `func EnvFromOS() Env` (spec defaults: `~/.local/share`, `/usr/local/share:/usr/share`, `~/.config`, `/etc/xdg`; `Desktops` from `XDG_CURRENT_DESKTOP` split on `:`, lower-cased).
  - `type Entry struct { ID, Name, Exec, TryExec string; Terminal, NoDisplay, Hidden, DBusActivatable bool; MimeTypes, Categories, OnlyShowIn, NotShowIn []string }`
  - `type Index struct { ... }`; `func Load(env Env, lookPath func(string) (string, error)) (*Index, error)`: reads `mime/globs2` from every data dir (weight order, case-insensitive suffix globs only; other glob shapes ignored), every `applications/**/*.desktop` (ID per spec: relative path with `/` → `-`; earlier dirs win), and `mimeapps.list` in the spec's order (`$XDG_CONFIG_HOME/<desktop>-mimeapps.list`, `$XDG_CONFIG_HOME/mimeapps.list`, the same in each config dir, then `applications/<desktop>-mimeapps.list` and `applications/mimeapps.list` in data home and data dirs). Entries that are `Hidden`, `NoDisplay`, fail `TryExec`, fail `OnlyShowIn`/`NotShowIn` against `Desktops`, or are `DBusActivatable` with empty `Exec` are dropped.
  - `func (ix *Index) TypeOf(name string) string`: MIME type for a file name or `.ext`; `application/octet-stream` when unknown.
  - `func (ix *Index) Handlers(mime string) []Entry`: `[Added Associations]` first in file order, then entries listing the type in `MimeType=`, minus `[Removed Associations]`, unique.
  - `func (ix *Index) Default(mime string) (Entry, bool)`: first `[Default Applications]` entry that exists, in lookup order.
  - `func ExpandExec(e Entry, path string) ([]string, error)`: Desktop Entry spec quoting (double-quoted args with `\"`, `` \` ``, `\$`, `\\` escapes; `%%` → `%`); `%f %F %u %U` → the path (the arg holding it is replaced whole); `%i` → `--icon`,`<Icon>` when set (Entry gains `Icon`); `%c` → Name; `%k` → the desktop file's path (Entry gains `File`); deprecated `%d %D %n %N %v %m` dropped; any other `%x` → error; no field code → path appended. Never a shell.

- [ ] **Step 1: Write the failing tests** with fixture trees: `TestTypeOfCaseAndMissing` (`a.PNG` → `image/png`; `README` → `application/octet-stream`; weight 60 beats 50); `TestHandlersOrderAndRemoved`; `TestDefaultDesktopSpecificFirst` (`XDG_CURRENT_DESKTOP=GNOME`: `gnome-mimeapps.list` beats `mimeapps.list`; a default naming a missing entry is skipped); `TestEntryFiltering` (each drop rule); `TestDesktopIDs` (`applications/kde/foo.desktop` → `kde-foo.desktop`; data home shadows data dirs); `TestExpandExec` (table: `code --new-window %F`, `"/opt/My App/bin/app" %u`, `sh -c "echo \"%f\""` → the path stays one argument inside its quoted word, `foo %%f %f`, `bar --icon %i`, `baz %x` → error, `qux` → path appended).
- [ ] **Step 2: Run** `go test ./internal/xdg/` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: read the XDG MIME, desktop entry and mimeapps files`

### Task 9: System() on Linux and the BSDs

**Files:**
- Create: `system/system_xdg.go` (`//go:build linux || freebsd || openbsd || netbsd`)
- Test: `system/system_xdg_test.go` (same build tag)

**Interfaces:**
- Consumes: `internal/xdg`, the cache, `ntopenwith.Command`/`Start`/`CheckPath`.
- Produces: the XDG `Source`. `Apps`: type from `t.Path` (name) or `t.Ext` via `TypeOf`; Open → `system` app (Name "System default", default when the index has none) plus `Handlers`, the index default first and marked; Edit → handlers whose `Categories` include any of `TextEditor`, `Graphics`, `Office`, `Development`, `3DGraphics`, never a default (so the Resolver goes to the chooser). IDs `xdg:<id>`. `Terminal` from the entry. `Launch`: `system` → `Start({"xdg-open", path})`; else find the entry again (`ErrNotOffered` if gone), `ExpandExec`, then `Start` or, for `Terminal`, the unstarted `*exec.Cmd`. A package var `loadIndex` lets tests point at fixtures.

- [ ] **Step 1: Write the failing tests** with a fixture tree: Open on `x.png` lists the mimeapps default first with `Default`; Edit lists only category matches and none is `Default`; `system` is offered for Open and launches `xdg-open` (startCommand hook); a removed entry is `ErrNotOffered` on Launch.
- [ ] **Step 2: Run** `GOOS=linux go vet ./system/` and, on Linux CI, `go test ./system/` — FAIL then.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `task cross` — PASS; the Ubuntu CI job runs the tests.
- [ ] **Step 5: Commit** `feat: System() for Linux and the BSDs`

### Task 10: Launch Services and System() on macOS

**Files:**
- Create: `internal/launchservices/ls.go` (types, any OS), `internal/launchservices/ls_darwin.go` (purego)
- Create: `system/system_darwin.go`
- Test: `internal/launchservices/ls_darwin_test.go`, `system/system_darwin_test.go`

**Interfaces:**
- Produces:
  - `type Role uint32`; `const Viewer Role = 0x2`, `Editor Role = 0x4` (kLSRoles values).
  - `type Bundle struct { ID, Name, Path string }`
  - `type Services interface { ForFile(path string, r Role) ([]Bundle, error); DefaultForFile(path string, r Role) (Bundle, bool, error); ForType(ext string, r Role) ([]Bundle, error); DefaultForType(ext string, r Role) (Bundle, bool, error) }`
  - `func New() (Services, error)` in `ls_darwin.go`: `purego.Dlopen` CoreFoundation and CoreServices; `RegisterLibFunc` for `CFURLCreateFromFileSystemRepresentation`, `CFURLGetFileSystemRepresentation`, `CFArrayGetCount`, `CFArrayGetValueAtIndex`, `CFRelease`, `CFStringCreateWithCString`, `CFStringGetCString`, `CFBundleCreate`, `CFBundleGetIdentifier`, `CFBundleGetValueForInfoDictionaryKey`, `LSCopyApplicationURLsForURL`, `LSCopyDefaultApplicationURLForURL`, `LSCopyAllRoleHandlersForContentType`, `LSCopyDefaultRoleHandlerForContentType`, `LSCopyApplicationURLsForBundleIdentifier`, `UTTypeCreatePreferredIdentifierForTag` (tag class `public.filename-extension`). Name: `CFBundleDisplayName`, else `CFBundleName`, else the bundle's file name without `.app`. Every returned CF object released.
  - The macOS `Source`: Open asks `Viewer|Editor`, Edit asks `Editor`; IDs `mac:<bundle id>` (bundles without an ID skipped); `system` app for Open (`Start({"open", path})`), default when Launch Services names none; Launch `Start({"open","-a",bundle.Path,path})` after re-checking the bundle is still offered. A package var `newServices` for a fake.

- [ ] **Step 1: Write the failing tests**: `TestServicesRealFile` (darwin only; a temp `.txt`: `ForFile(Viewer|Editor)` non-empty and includes `com.apple.TextEdit`; `DefaultForFile` ok; `ForType(".txt", Editor)` non-empty; each Bundle has ID, Name and an existing Path); `TestDarwinSourceWithFake` (fake Services: default first and `Default`, Edit uses `Editor`, `system` present for Open only, Launch argv via the startCommand hook, an un-offered bundle → `ErrNotOffered`).
- [ ] **Step 2: Run** `go test ./internal/launchservices/ ./system/` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS; `CGO_ENABLED=0 go build ./...` — PASS; `task cross` — PASS.
- [ ] **Step 5: Commit** `feat: System() for macOS through Launch Services`

### Task 11: Windows associations and System() on Windows

**Files:**
- Create: `internal/winassoc/assoc.go` (any OS, pure logic over an interface), `internal/winassoc/registry_windows.go`, `internal/winassoc/shell_windows.go`
- Create: `system/system_windows.go`
- Test: `internal/winassoc/assoc_test.go` (fake registry, any OS), `system/system_windows_test.go`

**Interfaces:**
- Produces:
  - `type Registry interface { String(root, path, name string) (string, error); ValueNames(root, path string) ([]string, error); SubKeys(root, path string) ([]string, error) }` with roots `"HKCU"`, `"HKLM"`, `"HKCR"`; `func OS() Registry` in `registry_windows.go` (`golang.org/x/sys/windows/registry`).
  - `type Handler struct { ID, ProgID, Exe string }`
  - `func Handlers(r Registry, ext, verb string) []Handler`: ProgIDs from `HKCU`/`HKLM` `Software\Classes\<ext>\OpenWithProgids` and `HKCR\<ext>\OpenWithProgids` (value names), plus `...\FileExts\<ext>\OpenWithList` executables (IDs `win:app:<exe>`); for `edit`, only ProgIDs with a `shell\edit\command` key, and no `OpenWithList` entries. IDs `win:<ProgID>`; unique.
  - `func Default(r Registry, ext, verb string) (Handler, bool)`: `open` → `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\<ext>\UserChoice` `ProgId`, else `HKCR\<ext>` default value; `edit` → that ProgID when it has `shell\edit`.
  - `func FriendlyName(progID, verb string) string` (`shell_windows.go`: `AssocQueryStringW` from `shlwapi.dll`, `ASSOCSTR_FRIENDLYAPPNAME`; falls back to the ProgID).
  - `func ShellExecute(path, verb, progID string) error` (`shell_windows.go`): `ShellExecuteExW` from `shell32.dll` with a `SHELLEXECUTEINFOW` declared here (x/sys has no ShellExecuteEx), `SEE_MASK_CLASSNAME` and `lpClass` when `progID != ""`, `SEE_MASK_NOASYNC`, `nShow = SW_SHOWNORMAL`.
  - The Windows `Source`: `system` app for Open (`ShellExecute(path, "open", "")`); handlers as Apps with `FriendlyName`; Edit default from `Default(..., "edit")`; `win:app:` entries launch with `Start({exe, path})` after `lookPath` finds the exe; Launch re-checks the offer.

- [ ] **Step 1: Write the failing tests**: `TestHandlersFromRegistry` and `TestDefault` (fake registry: UserChoice beats HKCR default; edit requires `shell\edit`; duplicates collapse; `.PNG` and `.png` the same); `TestWindowsSourceReal` (windows only: a temp `.txt` has at least one Open app and no error).
- [ ] **Step 2: Run** `go test ./internal/winassoc/` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/winassoc/` and `GOOS=windows go vet ./...` — PASS; the Windows CI job runs the real test.
- [ ] **Step 5: Commit** `feat: System() for Windows through the registry and the shell`

### Task 12: The ntopen example, documentation and notices

**Files:**
- Create: `examples/ntopen/main.go`, `README.md`, `AGENTS.md`, `DEVELOP.md`, `CHANGELOG.md`, `THIRD_PARTY_NOTICES.md`
- Modify: `Taskfile.yml` (`build`, `notices`, `notices-check` as ntfilepicker's)
- Test: `examples/ntopen/main_test.go`

**Interfaces:**
- Consumes: everything.
- Produces: `ntopen [list|open|edit] FILE [--app ID] [--json] [--text]` (stdlib `flag`). `list` prints Open and Edit choices from `NewResolver(Layered(system.System(), List(TerminalEditor()), List(TerminalEditors()...)), Preferences{})`, the resolved app marked `*`, as text or a JSON array of `{verb, id, name, default, terminal}`; `open`/`edit` resolve (or use `--app`) and launch, running a terminal app attached with the process's stdio; `ErrChoose` prints the choices and exits 2.

- [ ] **Step 1: Write the failing test** `TestListJSON`: run `run([]string{"list","--json",tmpTxt}, &out)` with a fake source injected through a package var; assert the JSON shape and the `default` flag.
- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement** `examples/ntopen/main.go` with `func run(args []string, stdout, stderr io.Writer) int`.
- [ ] **Step 4: Write the docs**: README in ntfilepicker's shape (what it is, the three sources with a short code sample each, Open/Edit rules table, safety rules, `ntopen`, and the same footer as `../ntfilepicker/README.md`); AGENTS.md (layout, conventions, the Global Constraints above); DEVELOP.md (Task, the CI matrix, testing each OS, purego notes); CHANGELOG with `## Unreleased`; notices via the notices tool as in ntfilepicker (`internal/tools/notices` copied and adjusted).
- [ ] **Step 5: Run** `task ci` and `task build && ./bin/ntopen list examples/ntopen/main.go` on this Mac — PASS, and the list shows TextEdit or VS Code with a default marked.
- [ ] **Step 6: Commit** `docs: README, guides and the ntopen example`

---

Gloss's move onto ntopenwith is a separate plan in gloss, after v0.1.0 is tagged here.
