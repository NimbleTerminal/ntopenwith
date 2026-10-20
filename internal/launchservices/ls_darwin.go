//go:build darwin

package launchservices

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

// The calls below follow Core Foundation's ownership rule: whatever a Create
// or Copy function returns is released here, and what a Get function returns
// is borrowed and never released. Only Go values leave this file.
//
// The role-based Launch Services calls are deprecated since macOS 12, but
// they are the only ones that answer by role (viewer or editor).

const (
	coreFoundation = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	coreServices   = "/System/Library/Frameworks/CoreServices.framework/CoreServices"

	encodingUTF8 = 0x08000100 // kCFStringEncodingUTF8
	maxPath      = 4096       // above PATH_MAX (1024)
	maxString    = 1 << 16    // the longest bundle ID or name read
)

// lib holds the C functions. CF types are uintptr, CFIndex is int, Boolean
// is uint8.
type lib struct {
	urlCreateFromFileSystemRepresentation func(alloc uintptr, buf []byte, n int, isDir uint8) uintptr
	urlGetFileSystemRepresentation        func(url uintptr, resolve uint8, buf []byte, n int) uint8
	arrayGetCount                         func(a uintptr) int
	arrayGetValueAtIndex                  func(a uintptr, i int) uintptr
	release                               func(ref uintptr)
	getTypeID                             func(ref uintptr) uint
	stringGetTypeID                       func() uint
	stringCreateWithCString               func(alloc uintptr, s string, enc uint32) uintptr
	stringGetCString                      func(s uintptr, buf []byte, n int, enc uint32) uint8
	bundleCreate                          func(alloc uintptr, url uintptr) uintptr
	bundleGetIdentifier                   func(b uintptr) uintptr
	bundleGetValueForInfoDictionaryKey    func(b uintptr, key uintptr) uintptr

	copyApplicationURLsForURL              func(url uintptr, r uint32) uintptr
	copyDefaultApplicationURLForURL        func(url uintptr, r uint32, outErr *uintptr) uintptr
	copyAllRoleHandlersForContentType      func(uti uintptr, r uint32) uintptr
	copyDefaultRoleHandlerForContentType   func(uti uintptr, r uint32) uintptr
	copyApplicationURLsForBundleIdentifier func(id uintptr, outErr *uintptr) uintptr
	typeCreatePreferredIdentifierForTag    func(class, tag, conformsTo uintptr) uintptr
}

var (
	loadOnce sync.Once
	loaded   *lib
	loadErr  error
)

// New loads Core Foundation and Core Services. It fails, rather than
// panics, when a framework or one of its functions is missing.
func New() (Services, error) {
	loadOnce.Do(func() { loaded, loadErr = load() })
	if loadErr != nil {
		return nil, loadErr
	}
	return loaded, nil
}

func load() (*lib, error) {
	cf, err := purego.Dlopen(coreFoundation, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("launchservices: %w", err)
	}
	cs, err := purego.Dlopen(coreServices, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("launchservices: %w", err)
	}
	l := new(lib)
	for _, f := range []struct {
		handle uintptr
		name   string
		fptr   any
	}{
		{cf, "CFURLCreateFromFileSystemRepresentation", &l.urlCreateFromFileSystemRepresentation},
		{cf, "CFURLGetFileSystemRepresentation", &l.urlGetFileSystemRepresentation},
		{cf, "CFArrayGetCount", &l.arrayGetCount},
		{cf, "CFArrayGetValueAtIndex", &l.arrayGetValueAtIndex},
		{cf, "CFRelease", &l.release},
		{cf, "CFGetTypeID", &l.getTypeID},
		{cf, "CFStringGetTypeID", &l.stringGetTypeID},
		{cf, "CFStringCreateWithCString", &l.stringCreateWithCString},
		{cf, "CFStringGetCString", &l.stringGetCString},
		{cf, "CFBundleCreate", &l.bundleCreate},
		{cf, "CFBundleGetIdentifier", &l.bundleGetIdentifier},
		{cf, "CFBundleGetValueForInfoDictionaryKey", &l.bundleGetValueForInfoDictionaryKey},
		{cs, "LSCopyApplicationURLsForURL", &l.copyApplicationURLsForURL},
		{cs, "LSCopyDefaultApplicationURLForURL", &l.copyDefaultApplicationURLForURL},
		{cs, "LSCopyAllRoleHandlersForContentType", &l.copyAllRoleHandlersForContentType},
		{cs, "LSCopyDefaultRoleHandlerForContentType", &l.copyDefaultRoleHandlerForContentType},
		{cs, "LSCopyApplicationURLsForBundleIdentifier", &l.copyApplicationURLsForBundleIdentifier},
		{cs, "UTTypeCreatePreferredIdentifierForTag", &l.typeCreatePreferredIdentifierForTag},
	} {
		sym, err := purego.Dlsym(f.handle, f.name)
		if err != nil {
			return nil, fmt.Errorf("launchservices: %s: %w", f.name, err)
		}
		purego.RegisterFunc(f.fptr, sym)
	}
	return l, nil
}

// fileURL makes a file URL of path, made absolute; the caller releases it.
func (l *lib) fileURL(path string) (uintptr, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return 0, fmt.Errorf("launchservices: invalid path %q", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	url := l.urlCreateFromFileSystemRepresentation(0, []byte(abs), len(abs), 0)
	if url == 0 {
		return 0, fmt.Errorf("launchservices: cannot make a URL of %s", abs)
	}
	return url, nil
}

// cfString makes a CFString of s; the caller releases it.
func (l *lib) cfString(s string) (uintptr, error) {
	if strings.ContainsRune(s, 0) {
		return 0, fmt.Errorf("launchservices: invalid string %q", s)
	}
	ref := l.stringCreateWithCString(0, s, encodingUTF8)
	if ref == 0 {
		return 0, errors.New("launchservices: cannot make a string")
	}
	return ref, nil
}

// goString copies a borrowed CFString, or returns "" for anything else.
func (l *lib) goString(ref uintptr) string {
	if ref == 0 || l.getTypeID(ref) != l.stringGetTypeID() {
		return ""
	}
	for n := 256; n <= maxString; n *= 2 {
		buf := make([]byte, n)
		if l.stringGetCString(ref, buf, n, encodingUTF8) != 0 {
			if i := bytes.IndexByte(buf, 0); i >= 0 {
				return string(buf[:i])
			}
			return ""
		}
	}
	return ""
}

// urlPath is the file system path of a borrowed URL, or "".
func (l *lib) urlPath(url uintptr) string {
	buf := make([]byte, maxPath)
	if l.urlGetFileSystemRepresentation(url, 1, buf, len(buf)) == 0 {
		return ""
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		return string(buf[:i])
	}
	return ""
}

// bundleAt describes the app at a borrowed URL. id is used when the bundle
// does not name its own.
func (l *lib) bundleAt(url uintptr, id string) Bundle {
	b := Bundle{ID: id, Path: l.urlPath(url)}
	if bundle := l.bundleCreate(0, url); bundle != 0 {
		defer l.release(bundle)
		if own := l.goString(l.bundleGetIdentifier(bundle)); own != "" {
			b.ID = own
		}
		for _, key := range []string{"CFBundleDisplayName", "CFBundleName"} {
			k, err := l.cfString(key)
			if err != nil {
				continue
			}
			b.Name = l.goString(l.bundleGetValueForInfoDictionaryKey(bundle, k))
			l.release(k)
			if b.Name != "" {
				break
			}
		}
	}
	if b.Name == "" {
		b.Name = strings.TrimSuffix(filepath.Base(b.Path), ".app")
	}
	return b
}

// bundlesAt describes the apps of an owned CFArray of URLs, releasing it.
func (l *lib) bundlesAt(urls uintptr) []Bundle {
	if urls == 0 {
		return nil
	}
	defer l.release(urls)
	var out []Bundle
	for i, n := 0, l.arrayGetCount(urls); i < n; i++ {
		if b := l.bundleAt(l.arrayGetValueAtIndex(urls, i), ""); b.Path != "" {
			out = append(out, b)
		}
	}
	return out
}

// appForID finds the app with this bundle ID (a borrowed CFString).
func (l *lib) appForID(id uintptr) (Bundle, bool) {
	var cfErr uintptr
	urls := l.copyApplicationURLsForBundleIdentifier(id, &cfErr)
	if cfErr != 0 {
		l.release(cfErr)
	}
	if urls == 0 {
		return Bundle{}, false
	}
	defer l.release(urls)
	if l.arrayGetCount(urls) == 0 {
		return Bundle{}, false
	}
	b := l.bundleAt(l.arrayGetValueAtIndex(urls, 0), l.goString(id))
	return b, b.Path != ""
}

// uti is the uniform type identifier for ext; the caller releases it.
func (l *lib) uti(ext string) (uintptr, error) {
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		return 0, errors.New("launchservices: empty extension")
	}
	class, err := l.cfString("public.filename-extension")
	if err != nil {
		return 0, err
	}
	defer l.release(class)
	tag, err := l.cfString(ext)
	if err != nil {
		return 0, err
	}
	defer l.release(tag)
	uti := l.typeCreatePreferredIdentifierForTag(class, tag, 0)
	if uti == 0 {
		return 0, fmt.Errorf("launchservices: no type for %q", ext)
	}
	return uti, nil
}

func (l *lib) ForFile(path string, r Role) ([]Bundle, error) {
	url, err := l.fileURL(path)
	if err != nil {
		return nil, err
	}
	defer l.release(url)
	return l.bundlesAt(l.copyApplicationURLsForURL(url, uint32(r))), nil
}

func (l *lib) DefaultForFile(path string, r Role) (Bundle, bool, error) {
	url, err := l.fileURL(path)
	if err != nil {
		return Bundle{}, false, err
	}
	defer l.release(url)
	var cfErr uintptr
	app := l.copyDefaultApplicationURLForURL(url, uint32(r), &cfErr)
	if cfErr != 0 {
		l.release(cfErr)
	}
	if app == 0 {
		return Bundle{}, false, nil
	}
	defer l.release(app)
	b := l.bundleAt(app, "")
	return b, b.Path != "", nil
}

func (l *lib) ForType(ext string, r Role) ([]Bundle, error) {
	uti, err := l.uti(ext)
	if err != nil {
		return nil, err
	}
	defer l.release(uti)
	ids := l.copyAllRoleHandlersForContentType(uti, uint32(r))
	if ids == 0 {
		return nil, nil
	}
	defer l.release(ids)
	var out []Bundle
	for i, n := 0, l.arrayGetCount(ids); i < n; i++ {
		// A handler that is registered but no longer installed has no app.
		if b, ok := l.appForID(l.arrayGetValueAtIndex(ids, i)); ok {
			out = append(out, b)
		}
	}
	return out, nil
}

func (l *lib) DefaultForType(ext string, r Role) (Bundle, bool, error) {
	uti, err := l.uti(ext)
	if err != nil {
		return Bundle{}, false, err
	}
	defer l.release(uti)
	id := l.copyDefaultRoleHandlerForContentType(uti, uint32(r))
	if id == 0 {
		return Bundle{}, false, nil
	}
	defer l.release(id)
	b, ok := l.appForID(id)
	return b, ok, nil
}
