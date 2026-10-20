// Package ntopenwith lists the apps that can open or edit a file, remembers
// which one a person prefers, and launches it without a shell.
//
// A [Target] names a file (or a folder) and a [Verb] says what to do
// with it. Apps come from three kinds of [Source]: the operating system's own
// registry of handlers (the System source), a fixed list the program compiles
// in (the List source), and the terminal editor named by $VISUAL or $EDITOR.
// Sources can be layered, so the program's own choices come before the system's.
//
// A launch is never a shell command. A graphical app is started through the
// operating system's launcher or an expanded desktop entry; a terminal app is
// returned as an unstarted [exec.Cmd] for the caller to run, since only the
// caller knows how to hand over its terminal.
//
// ntopenwith has no user interface. A program draws its own menu from the
// apps a Source reports, and stores the person's choice as an app ID.
package ntopenwith
