package main

import (
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// memFS is a read-only tree of names and sizes with no contents: enough for
// the chooser to list. (ntfilepicker has one of its own, but internal.)
type memFS struct {
	nodes map[string]*memNode // by slash path from the root, "." for the root
}

// memNode is a file or folder; it is its own fs.FileInfo and fs.DirEntry.
type memNode struct {
	name     string
	size     int64
	dir      bool
	children []*memNode // sorted by name
}

// modTime is the one date every pretend file has.
var modTime = time.Date(2026, 10, 19, 9, 41, 0, 0, time.UTC)

// add puts a file, or with dir a folder, at name, making the folders above it.
func (f *memFS) add(name string, size int64, dir bool) *memNode {
	name = path.Clean(name)
	if n, ok := f.nodes[name]; ok {
		return n
	}
	parent := f.add(path.Dir(name), 0, true)
	n := &memNode{name: path.Base(name), size: size, dir: dir}
	f.nodes[name] = n
	i, _ := slices.BinarySearchFunc(parent.children, n.name, func(c *memNode, s string) int { return strings.Compare(c.name, s) })
	parent.children = slices.Insert(parent.children, i, n)
	return n
}

func (f *memFS) lookup(op, name string) (*memNode, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	n, ok := f.nodes[name]
	if !ok {
		return nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	return n, nil
}

func (f *memFS) Open(name string) (fs.File, error) {
	n, err := f.lookup("open", name)
	if err != nil {
		return nil, err
	}
	return &memFile{node: n}, nil
}

func (f *memFS) Stat(name string) (fs.FileInfo, error) { return f.lookup("stat", name) }

func (f *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	n, err := f.lookup("readdir", name)
	if err != nil {
		return nil, err
	}
	if !n.dir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	out := make([]fs.DirEntry, len(n.children))
	for i, c := range n.children {
		out[i] = c
	}
	return out, nil
}

func (n *memNode) Name() string { return n.name }
func (n *memNode) Size() int64  { return n.size }
func (n *memNode) Mode() fs.FileMode {
	if n.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (n *memNode) ModTime() time.Time         { return modTime }
func (n *memNode) IsDir() bool                { return n.dir }
func (n *memNode) Sys() any                   { return nil }
func (n *memNode) Type() fs.FileMode          { return n.Mode().Type() }
func (n *memNode) Info() (fs.FileInfo, error) { return n, nil }

// memFile is an open node. Reading a file gives nothing: there are no contents.
type memFile struct {
	node *memNode
	read int // entries already returned by ReadDir
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.node, nil }
func (f *memFile) Read([]byte) (int, error)   { return 0, io.EOF }
func (f *memFile) Close() error               { return nil }

func (f *memFile) ReadDir(count int) ([]fs.DirEntry, error) {
	if !f.node.dir {
		return nil, &fs.PathError{Op: "readdir", Path: f.node.name, Err: fs.ErrInvalid}
	}
	rest := f.node.children[f.read:]
	if count > 0 {
		if len(rest) == 0 {
			return nil, io.EOF
		}
		rest = rest[:min(count, len(rest))]
	}
	f.read += len(rest)
	out := make([]fs.DirEntry, len(rest))
	for i, c := range rest {
		out[i] = c
	}
	return out, nil
}
