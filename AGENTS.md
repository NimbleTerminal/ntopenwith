# AGENTS.md

## What ntopenwith is

A library that lists the apps able to open or edit a file, picks one (the OS
default first, the person's saved choice over it) and launches it safely without
a shell. Module `nimbleterminal.dev/ntopenwith`, Go 1.26.8+. README is the
introduction, DEVELOP.md is for working on it; keep them, this file and
CHANGELOG consistent with the code. Written for gloss to move onto
(`internal/openwith` there), and knows nothing of gloss.

## Layout

- Root package `ntopenwith`: `Target`, `Verb` (`Open`, `Edit`), `App`, `Source`;
  `List`/`ListApp`, `TerminalEditor(s)`, `Layered` with its corrections (`Hide`,
  `Rename`, `Rank`, `Alias`, `Override`); `Preferences`; `Resolver`; `Reveal`;
  `Command` and `Start` (argv without a shell, detached start).
- `system`: `System()` and `Refresh()`. Linux/BSD XDG, macOS Launch Services,
  Windows registry and shell, and an empty source elsewhere (including js/wasm).
  The per-OS logic is in untagged files (`xdg_source.go`, `mac_source.go`,
  `win_source.go`) over small interfaces, so its tests run on every OS; only the
  constructors and the OS calls are build-tagged. `cache.go` keeps listings five
  seconds.
- `internal/xdg`: MIME globs, desktop entries, `mimeapps.list`, `Exec` expansion.
  `internal/launchservices`: Launch Services through purego, behind an
  interface. `internal/winassoc`: the registry and `ShellExecuteExW`, behind an
  interface.
- `examples/ntopen`: the example command, in the root module. It takes its
  source from a package variable so the test can inject a fake.
- `internal/tools/notices`: `THIRD_PARTY_NOTICES.md` for every OS.

## Global constraints

- Root `go.mod` requires only purego, `x/sys` and shlex; the rest is the
  standard library. `CGO_ENABLED=0` must build on every OS. Under `js/wasm`
  everything builds, `System()` lists nothing, and every launch returns
  `ErrUnsupported`.
- No user interface and no Bubble Tea. A terminal app comes back as an
  unstarted `*exec.Cmd`; the library never starts one.
- The library never invokes a shell (a desktop entry's own `Exec` may name one;
  README, Safety). What runs: an OS handler through the OS's launcher (`open -a`,
  `ShellExecuteExW`), an XDG `Exec` expanded per the Desktop Entry
  specification, a `List` command compiled into the program, or
  `$VISUAL`/`$EDITOR`. Nothing else, and nothing from a preference file, a
  file name, or any environment variable other than `VISUAL`/`EDITOR` can add a
  command. `PATH`, `HOME` (`USERPROFILE` on Windows), `PATHEXT` and the `XDG_*`
  variables shape the OS's own list, as for any file manager.
- Preferences hold IDs only; an ID launches only if the source still offers it
  for that target and verb. `Layered` launches only what it offers now.
- Paths launched are absolute, cleaned, NUL-free, a regular file (a directory
  only for `Reveal` or a `Directory` target), and passed as one argument.
- Edit on a non-`Text` target never offers or uses `$VISUAL`/`$EDITOR`.
- Preference files: at most 64 KiB, one JSON object, unknown fields and trailing
  data rejected, written to a temporary file in the same folder and renamed,
  folder created `0700`.
- App IDs: `system`, `terminal-editor`, `mac:<bundle id>`,
  `xdg:<desktop file id>`, `win:<ProgID>`, `win:app:<exe name>`; `List` IDs are
  the developer's.
- Decided in review, and documented as such: XDG has no Edit default
  (`ErrChoose`); an XDG `Exec` with a file code inside quotes, more than one file
  code, or a field code as the program is refused; Windows offers only `.exe`
  programs from `OpenWithList` whose `Applications\<exe>` class registers the
  verb, launched through that class; "System default" runs the OS's own opener, which
  runs executable file types, so programs must filter them (README, Safety).

## Conventions

- gofmt, a small public API, comments in plain words. Follow the gloss code
  style and the layout of `../ntfilepicker`.
- Names from the system are data; the library returns them unescaped and says
  so. The example escapes them.
- Tests use fakes and fixture trees, not the machine, except the CI tests that
  ask the real OS and assert only what every runner has.
- Taskfile sets `GOWORK: 'off'` and builds use `-mod=readonly`.

## Verification

`task ci` before finishing: gofmt, `go mod tidy -diff`, race tests, vet, the
cross-platform builds without CGO (windows, linux, darwin, freebsd, js/wasm), the
example, the notices check, staticcheck. If staticcheck fails on a Go toolchain
mismatch, rerun it with `GOTOOLCHAIN` set to the version in `go.mod`. CI runs it
on macOS, Ubuntu and Windows.
