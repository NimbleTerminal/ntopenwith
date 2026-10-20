// Command choose is a complete host for ntopenwith, built with Bubble Tea and
// ntfilepicker: choose a file, then open or edit it with the app the system
// (or your saved choice) picks, or choose the app yourself.
//
//	go run ./choose                 # your files, from your home folder
//	go run ./choose -pretend        # a made-up home folder; nothing launches
//	go run ./choose -prefs FILE     # keep the defaults you save in FILE
//
// Enter opens the file, Ctrl+E edits it, Alt+Enter and Alt+E choose the app
// to open or edit it with. In the chooser, d saves the highlighted app as the
// default for the file's type. Under js/wasm it is always pretend.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"charm.land/bubbles/v2/key"
	"nimbleterminal.dev/ntfilepicker"
	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/examples/internal/demorun"
	"nimbleterminal.dev/ntopenwith/system"
)

// imageEdit hides QuickTime Player from editing images: macOS lists it as an
// image editor, and it is not one. It still opens them.
func imageEdit() ntopenwith.Source {
	return ntopenwith.HideFor([]ntopenwith.Verb{ntopenwith.Edit},
		[]string{".png", ".jpg", ".jpeg", ".gif", ".heic"}, "mac:com.apple.QuickTimePlayerX")
}

// nativeSource is the system's apps, corrected, then the terminal editors.
func nativeSource() ntopenwith.Source {
	return ntopenwith.Layered(
		system.System(),
		imageEdit(),
		ntopenwith.List(ntopenwith.TerminalEditor()),
		ntopenwith.List(ntopenwith.TerminalEditors()...),
	)
}

// pretendConfig is the made-up home folder with apps from a table.
func pretendConfig(layout ntfilepicker.Layout) config {
	tree := pretendTree()
	p := newPretend(tree)
	return config{
		fsys:    tree,
		home:    pretendHome,
		source:  ntopenwith.Layered(p, imageEdit()),
		layout:  layout,
		pretend: p,
	}
}

// nativeConfig is the real files from dir, and the system's apps.
func nativeConfig(layout ntfilepicker.Layout, dir, prefsFile string) (config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return config{}, err
	}
	// The chooser's paths are slash-separated from the filesystem's root.
	volume := filepath.VolumeName(abs)
	c := config{
		fsys:   os.DirFS(volume + string(filepath.Separator)).(fs.ReadDirFS),
		home:   filepath.ToSlash(abs[len(volume):]),
		volume: volume,
		source: nativeSource(),
		layout: layout,
	}
	if prefsFile != "" {
		if c.prefs, err = ntopenwith.LoadPreferences(prefsFile); err != nil {
			return config{}, err
		}
		c.save = func(p ntopenwith.Preferences) error { return p.Save(prefsFile) }
	}
	return c, nil
}

func main() {
	layoutName := flag.String("layout", "list", "the layout to start in: list, columns, or places")
	pretend := flag.Bool("pretend", false, "use a made-up home folder and apps; launch nothing")
	prefsFile := flag.String("prefs", "", "load and save the defaults in this file (else they are kept in memory)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: choose [-pretend] [-prefs FILE] [-layout NAME] [FOLDER]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	layout, ok := ntfilepicker.ParseLayout(*layoutName)
	if !ok || flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	var c config
	if *pretend || runtime.GOOS == "js" {
		c = pretendConfig(layout)
	} else {
		dir, err := os.UserHomeDir()
		if flag.NArg() == 1 {
			dir, err = flag.Arg(0), nil
		}
		if err == nil {
			c, err = nativeConfig(layout, dir, *prefsFile)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "choose:", err)
			os.Exit(1)
		}
	}

	b := demorun.Binding
	if err := demorun.Run(newModel(c), "browse", "Open or edit a file", demorun.Keys{
		Primary: []key.Binding{b("enter", "Open"), b("ctrl+e", "Edit"), b("alt+enter", "Open with"), b("alt+e", "Edit with")},
		More:    []key.Binding{b("alt+up", "Up a folder"), b("ctrl+l", "Layout"), b("esc", "Clear filter"), b("ctrl+c", "Quit")},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "choose:", err)
		os.Exit(1)
	}
}
