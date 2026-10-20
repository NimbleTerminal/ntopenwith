package xdg

import (
	"bufio"
	"bytes"
	"path/filepath"
	"slices"
	"strings"
)

const (
	maxMimeInfoSize = 16 << 20
	maxMimeDepth    = 64
	maxMimeTypes    = 4096
)

// readMimePairs reads aliases or subclasses, each a pair of MIME types per
// line, with the same file and line bounds as the other MIME readers.
func readMimePairs(dir, file string, add func(string, string)) {
	data, ok := readBounded(filepath.Join(dir, "mime", file), maxMimeInfoSize)
	if !ok {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, maxLineSize)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.ToLower(line))
		if len(fields) == 2 && strings.Contains(fields[0], "/") && strings.Contains(fields[1], "/") {
			add(fields[0], fields[1])
		}
	}
}

func (ix *Index) readInheritance(dirs []string) {
	// Aliases must be complete before any other MIME keys are canonicalized.
	for _, dir := range dirs {
		readMimePairs(dir, "aliases", func(alias, canonical string) {
			if _, exists := ix.aliases[alias]; !exists {
				ix.aliases[alias] = canonical
			}
		})
	}
	seen := map[[2]string]bool{}
	for _, dir := range dirs {
		readMimePairs(dir, "subclasses", func(child, parent string) {
			child, parent = ix.canonical(child), ix.canonical(parent)
			pair := [2]string{child, parent}
			if !seen[pair] {
				seen[pair] = true
				ix.parents[child] = append(ix.parents[child], parent)
			}
		})
	}
}

func (ix *Index) canonical(mime string) string {
	mime = strings.ToLower(mime)
	var path []string
	for range maxMimeDepth {
		next := ix.aliases[mime]
		if next == "" {
			return mime
		}
		if i := slices.Index(path, mime); i >= 0 {
			// Give every member of an alias cycle the same stable name.
			return slices.Min(path[i:])
		}
		path = append(path, mime)
		mime = next
	}
	return mime
}

// ancestry returns the type itself, then its parents breadth first. Text
// implicitly inherits text/plain. Octet-stream is useful only as the queried
// type: climbing to it would offer apps for arbitrary binary data.
func (ix *Index) ancestry(mime string) []string {
	mime = ix.canonical(mime)
	out := []string{mime}
	if mime == unknownType {
		return out
	}
	seen := map[string]bool{mime: true}
	level := []string{mime}
	for depth := 0; depth < maxMimeDepth && len(level) > 0; depth++ {
		var next []string
		for _, child := range level {
			add := func(parent string) {
				if parent != unknownType && !seen[parent] && len(out) < maxMimeTypes {
					seen[parent] = true
					out = append(out, parent)
					next = append(next, parent)
				}
			}
			for _, parent := range ix.parents[child] {
				add(parent)
			}
			if strings.HasPrefix(child, "text/") {
				add(ix.canonical("text/plain"))
			}
		}
		level = next
	}
	return out
}
