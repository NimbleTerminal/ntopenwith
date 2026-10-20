package xdg

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
)

const unknownType = "application/octet-stream"

// glob is one case-insensitive suffix pattern from a globs2 file.
type glob struct {
	mime   string
	weight int
}

// readGlobs adds the suffix globs of dir/mime/globs2 to into. Lines are
// weight:type:glob[:flags]. Only globs of the form *literal are kept, and not
// those flagged "cs" (case-sensitive): everything else is ignored. For one
// suffix the higher weight wins, and the earlier file wins a tie.
func readGlobs(dir string, into map[string]glob) {
	data, ok := readBounded(filepath.Join(dir, "mime", "globs2"), maxGlobsSize)
	if !ok {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, maxLineSize)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 3 {
			continue
		}
		weight, err := strconv.Atoi(parts[0])
		if err != nil || parts[1] == "" {
			continue
		}
		if len(parts) == 4 && strings.Contains(parts[3], "cs") {
			continue
		}
		suffix, ok := suffixGlob(parts[2])
		if !ok {
			continue
		}
		if old, dup := into[suffix]; dup && old.weight >= weight {
			continue
		}
		into[suffix] = glob{mime: strings.ToLower(parts[1]), weight: weight}
	}
}

// suffixGlob returns the lower-cased literal of a "*literal" glob.
func suffixGlob(g string) (string, bool) {
	if len(g) < 2 || g[0] != '*' || strings.ContainsAny(g[1:], `*?[]\`) {
		return "", false
	}
	return strings.ToLower(g[1:]), true
}

// TypeOf returns the MIME type for a file name or a bare extension such as
// ".png", matching globs without regard to case. Of the globs that match, the
// highest weight wins, then the longest. Aliases resolve to the canonical type. An unknown name is
// application/octet-stream. Only the globs are consulted, never the file's
// content.
func (ix *Index) TypeOf(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(name)
	best, bestLen, found := glob{}, 0, false
	for i := range name {
		g, ok := ix.globs[name[i:]]
		if !ok {
			continue
		}
		n := len(name) - i
		if !found || g.weight > best.weight || (g.weight == best.weight && n > bestLen) {
			best, bestLen, found = g, n, true
		}
	}
	if !found {
		return unknownType
	}
	return ix.canonical(best.mime)
}
