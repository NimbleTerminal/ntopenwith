# `ntopenwith` CHANGELOG

## Unreleased

First version.

* Add `Target`, `Verb` (`Open`, `Edit`), `App` and `Source`: which apps can open
  or edit a file, and how to launch one without a shell.
* Add `system.System()`: the operating system's own handlers and defaults, with
  Edit as a verb of its own. macOS through Launch Services (no CGO), Linux and
  the BSDs through the XDG files, Windows through the registry and the shell;
  empty on other platforms and under `js/wasm`. `system.Refresh()` drops the
  five-second listings.
* Add `List`, `ListApp`, `TerminalEditor` and `TerminalEditors`: a program's own
  apps, and the editor named by `$VISUAL` or `$EDITOR` (for text only).
* Add `Layered` with `Hide`, `HideFor`, `Rename`, `Rank`, `Alias` and `Override`: one source
  corrected by another, with old app IDs kept as aliases.
* Add `Preferences` (IDs only) with a bounded file format, and
  `LoadLegacyPreferences` for gloss's earlier one.
* Add `Resolver`: the saved choice, then the OS default, for Open and Edit;
  `ErrChoose` when Edit has no default, `ErrNoEditor` when nothing can edit the
  file; `Choices`, `AllApps`, `SetDefault`, `SetHidden`.
* Add `Reveal`, `Command` and `Start`.
* Add `examples/ntopen`, a command that lists, opens and edits with the above.
* Add `examples/choose`, a Bubble Tea host on ntfilepicker that opens and edits
  files and shows the "Open with" and "Edit with" choosers, with a `-pretend`
  mode over a made-up home folder. The examples are now a module of their own.
* Add a browser demo of `examples/choose`, at
  <https://nimbleterminal.github.io/ntopenwith/>.
