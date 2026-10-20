// ntopen tries ntopenwith on this machine: it lists the apps that can open or
// edit a file, and opens or edits it with one.
//
//	ntopen [list|open|edit] FILE [--app ID] [--json] [--text]
//
// With no command it lists. The apps come from the operating system, then the
// terminal editors: $VISUAL or $EDITOR, and the common ones found on the PATH.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/system"
)

// source supplies the apps; tests replace it.
var source = func() ntopenwith.Source {
	return ntopenwith.Layered(
		system.System(),
		ntopenwith.List(ntopenwith.TerminalEditor()),
		ntopenwith.List(ntopenwith.TerminalEditors()...),
	)
}

// Exit codes: 2 is for a choice the program could not make for you.
const (
	exitOK     = 0
	exitFailed = 1
	exitChoose = 2
)

const usage = `usage: ntopen [list|open|edit] FILE [--app ID] [--json] [--text]

  list   show the apps that can open and edit FILE; * marks the one used by default
  open   open FILE with the default app, or with --app ID
  edit   edit FILE with the default editor, or with --app ID

  --app ID  use the app with this ID (see list)
  --json    print JSON
  --text    treat FILE as text, so that a terminal editor may edit it

Exit status: 0 done, 1 failed, 2 no default editor: choose one with --app.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// entry is one app in the JSON output.
type entry struct {
	Verb     ntopenwith.Verb `json:"verb"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Default  bool            `json:"default"`
	Terminal bool            `json:"terminal"`
}

func run(args []string, stdout, stderr io.Writer) int {
	command := "list"
	if len(args) > 0 && slices.Contains([]string{"list", "open", "edit"}, args[0]) {
		command, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("ntopen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	appID := fs.String("app", "", "")
	asJSON := fs.Bool("json", false, "")
	asText := fs.Bool("text", false, "")
	// Flags may come before or after the file.
	var files []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Fprint(stdout, usage)
				return exitOK
			}
			fmt.Fprintf(stderr, "ntopen: %v\n%s", err, usage)
			return exitFailed
		}
		if fs.NArg() == 0 {
			break
		}
		files, args = append(files, fs.Arg(0)), fs.Args()[1:]
	}
	if len(files) != 1 {
		fmt.Fprint(stderr, usage)
		return exitFailed
	}

	path, err := ntopenwith.CheckPath(files[0], false)
	if err != nil {
		fmt.Fprintf(stderr, "ntopen: %v\n", err)
		return exitFailed
	}
	target := ntopenwith.Target{Path: path, Text: *asText || looksLikeText(path)}
	r := ntopenwith.NewResolver(source(), ntopenwith.Preferences{})
	ctx := context.Background()

	if command == "list" {
		var all []entry
		for _, verb := range []ntopenwith.Verb{ntopenwith.Open, ntopenwith.Edit} {
			apps, err := r.Choices(ctx, target, verb)
			if err != nil {
				fmt.Fprintf(stderr, "ntopen: %v\n", err)
				return exitFailed
			}
			all = append(all, entries(verb, apps)...)
		}
		return printList(stdout, stderr, []ntopenwith.Verb{ntopenwith.Open, ntopenwith.Edit}, all, *asJSON)
	}

	verb := ntopenwith.Open
	if command == "edit" {
		verb = ntopenwith.Edit
	}
	apps, err := r.Choices(ctx, target, verb)
	if err != nil {
		fmt.Fprintf(stderr, "ntopen: %v\n", err)
		return exitFailed
	}
	var app ntopenwith.App
	if *appID != "" {
		i := slices.IndexFunc(apps, func(a ntopenwith.App) bool { return a.ID == *appID })
		if i < 0 {
			fmt.Fprintf(stderr, "ntopen: %s: %v for %s\n", *appID, ntopenwith.ErrNotOffered, verb)
			return exitFailed
		}
		app = apps[i]
	} else {
		choice, err := r.Resolve(ctx, target, verb)
		if errors.Is(err, ntopenwith.ErrChoose) {
			fmt.Fprintf(stderr, "ntopen: %v\n", err)
			printList(stdout, stderr, []ntopenwith.Verb{verb}, entries(verb, apps), *asJSON)
			return exitChoose
		}
		if err != nil {
			fmt.Fprintf(stderr, "ntopen: %v\n", err)
			return exitFailed
		}
		app = choice.App
	}

	// The command is started by the caller when the app is a terminal one.
	cmd, err := r.Launch(ctx, target, verb, app)
	if err != nil {
		fmt.Fprintf(stderr, "ntopen: %v\n", err)
		return exitFailed
	}
	if cmd != nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "ntopen: %v\n", err)
			return exitFailed
		}
		return exitOK
	}
	if *asJSON {
		out, _ := json.Marshal(entries(verb, []ntopenwith.App{app})[0])
		fmt.Fprintf(stdout, "%s\n", out)
	} else {
		fmt.Fprintf(stdout, "%s %s with %s (%s)\n", map[ntopenwith.Verb]string{ntopenwith.Open: "opened", ntopenwith.Edit: "editing"}[verb], target.Path, clean(app.Name), app.ID)
	}
	return exitOK
}

// entries describes apps for printing.
func entries(v ntopenwith.Verb, apps []ntopenwith.App) []entry {
	out := make([]entry, 0, len(apps))
	for _, a := range apps {
		out = append(out, entry{Verb: v, ID: a.ID, Name: a.Name, Default: a.Default, Terminal: a.Terminal})
	}
	return out
}

// printList writes the entries as text or JSON.
func printList(stdout, stderr io.Writer, verbs []ntopenwith.Verb, list []entry, asJSON bool) int {
	if asJSON {
		if list == nil {
			list = []entry{}
		}
		out, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "ntopen: %v\n", err)
			return exitFailed
		}
		fmt.Fprintf(stdout, "%s\n", out)
		return exitOK
	}
	for i, verb := range verbs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintf(stdout, "%s:\n", verb)
		n := 0
		for _, e := range list {
			if e.Verb != verb {
				continue
			}
			n++
			mark, kind := " ", ""
			if e.Default {
				mark = "*"
			}
			if e.Terminal {
				kind = "  (terminal)"
			}
			fmt.Fprintf(stdout, "  %s %s  %s%s\n", mark, e.ID, clean(e.Name), kind)
		}
		if n == 0 {
			fmt.Fprintln(stdout, "  (none)")
		}
	}
	return exitOK
}

// clean makes a name from the system safe to print: control characters,
// which could carry escape sequences, become '?'.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}

// looksLikeText reports whether the start of the file is valid UTF-8 without
// NUL bytes.
func looksLikeText(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	const size = 8192
	buf := make([]byte, size)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if bytes.IndexByte(buf, 0) >= 0 {
		return false
	}
	// A full buffer may end inside a character: drop its partial end.
	if n == size {
		for i := 0; i < utf8.UTFMax-1 && !utf8.Valid(buf); i++ {
			buf = buf[:len(buf)-1]
		}
	}
	return utf8.Valid(buf)
}
