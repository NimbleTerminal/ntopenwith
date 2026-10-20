# Developing ntopenwith

A library with no UI. The root package is the model (targets, verbs, apps,
sources, preferences, the resolver) and `system` is the one source that reads
the operating system. See [AGENTS.md](AGENTS.md) for the layout and the rules a
change must keep, and the README for what it promises.

## Tasks

[Task](https://taskfile.dev) runs everything, with `GOWORK=off`:

```sh
task test             # go test ./...
task test-race        # with the race detector
task vet
task cross            # vet/build for windows, linux, darwin, freebsd, js/wasm, CGO off
task build            # bin/ntopen, the example
task notices          # rewrite THIRD_PARTY_NOTICES.md
task ci               # the gate
```

`task ci` runs: gofmt check, `go mod tidy -diff`, race tests, vet, the
cross-platform builds, the example build, the notices check, and staticcheck
(fetched at its latest version, which may need the newest Go toolchain; if it
fails on a mismatch, rerun it with `GOTOOLCHAIN` set to the `go.mod` version).
GitHub Actions runs `task ci` on `ubuntu-latest`, `macos-latest` and
`windows-latest`, so the real-OS tests below run on all three.

## Testing

- **Units with fakes.** The XDG, macOS and Windows sources keep their logic in
  untagged files over interfaces (`internal/xdg`'s index, `launchservices.Services`,
  `winassoc`'s registry), so their tests run on any OS. XDG reads fixture trees
  under `internal/xdg/testdata`: `mimeapps.list` precedence, removed associations,
  `Exec` field codes and quoting, `TryExec`, `Terminal=true`.
- **Resolver tables** cover every row of "Choosing and launching" in the README,
  including the binary file with only `$EDITOR` set, which must not use it.
- **Launch tests** use a `List` app that is a test helper binary recording its
  arguments, to check that the path arrives as one argument, unchanged.
- **The real OS.** `system_darwin_test.go` and `system_windows_test.go` (selected
  by their file names) ask the machine and assert only what every runner has:
  that a `.txt` has an Open handler and that queries return without error.
  Linux has no such test yet: its source is tested against the fixture trees.
- **The example** is tested through `run` with a fake source injected through
  the `source` variable.

### Trying each OS

You need the OS itself for the real-OS tests; CI has all three. Locally,
`task cross` proves that every OS builds. To look at a machine, build the example
and ask it:

```sh
task build
bin/ntopen list some.png
bin/ntopen list --json some.txt
```

On Linux, `XDG_DATA_HOME`, `XDG_DATA_DIRS`, `XDG_CONFIG_HOME`, `XDG_CONFIG_DIRS`
and `XDG_CURRENT_DESKTOP` change what it sees, as they change a file manager;
point them at a fixture tree to reproduce a report. `system.Refresh()` is not
needed between runs of a command, only inside one process, which keeps answers
five seconds.

## purego on macOS

Launch Services and Core Foundation are C APIs. The module calls them through
[purego](https://github.com/ebitengine/purego), which needs no CGO: it opens
`CoreFoundation.framework` and `CoreServices.framework` at run time and registers
the functions it uses (`internal/launchservices/ls_darwin.go`). A few things to
keep in mind:

- The calls used (`LSCopyApplicationURLsForURL`, `LSCopyDefaultApplicationURLForURL`)
  are deprecated since macOS 12 but are the only ones that take a *role*, which
  is what separates Open from Edit. Their replacements in NSWorkspace take none.
  They sit behind one small interface (`launchservices.Services`) so they can be
  replaced.
- Core Foundation objects that a `Copy` function returns are owned by the caller:
  release them (`CFRelease`) on every path, including errors.
- `internal/launchservices/ls.go` builds on every OS (the types and the
  interface); only `ls_darwin.go` holds the purego calls, so the rest can be
  tested with a fake anywhere. If Launch Services cannot be loaded, `System()`
  falls back to the empty source.
- Real data is noisy: QuickTime Player claims to "edit" PNG, and some apps only
  "view" a format. That is what `Layered` corrections (`HideFor`, `Hide`, `Rank`)
  are for in the program, not something to patch in the library:
  `HideFor([]Verb{Edit}, []string{".png"}, "mac:com.apple.QuickTimePlayerX")`
  keeps QuickTime for opening a PNG and drops it from editing one.

## Windows

The registry and `ShellExecuteExW` come from `golang.org/x/sys/windows`. Handlers
are read from the extension's `UserChoice`, `OpenWithProgids` and `OpenWithList`
(user and machine); the ProgID's `shell\edit` verb gives Edit. An `OpenWithList`
program (`win:app:<exe>`, a bare `*.exe` name) is listed only when its class,
`HKCR\Applications\<exe>`, registers `shell\open\command` (for Edit,
`shell\edit\command`): that is where GUI programs register, not the `PATH`.
It is launched with `ShellExecuteExW` and `SEE_MASK_CLASSNAME` on that class, so
Windows runs the command the program registered; the library builds no command
line and never starts the program itself, so a batch file cannot be reached.

## Releasing

Update CHANGELOG.md, run `task ci`, and tag `vX.Y.Z`. The tag is pushed by a
person; nothing here publishes.
