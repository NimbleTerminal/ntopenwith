package ntopenwith

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"unicode"
)

const (
	// maxPreferences is the largest preference file read or written.
	maxPreferences = 64 << 10
	// maxPreferenceString is the longest key or ID kept.
	maxPreferenceString = 256
	preferencesVersion  = 1
)

// Preferences holds a person's choices of app, as IDs only. An ID launches
// only if the Source still offers it for the target and verb.
type Preferences struct {
	// Version is the file format; Save sets it to 1 and Load requires it.
	Version int `json:"version"`
	// Defaults maps a verb, then a target's Key, to the ID of the chosen app.
	Defaults map[Verb]map[string]string `json:"defaults"`
	// Hidden lists IDs of apps the person does not want offered.
	Hidden []string `json:"hidden"`
}

// legacyPreferences is the file gloss wrote before ntopenwith.
type legacyPreferences struct {
	Defaults map[string]string `json:"defaults"`
	Hidden   []string          `json:"hidden"`
}

// LoadPreferences reads a preference file. A missing file gives empty
// preferences. The file must be at most 64 KiB, one JSON object with version
// 1, no unknown fields and no trailing data, and may name only the verbs
// "open" and "edit". Entries whose key or ID is empty, over 256 bytes or holds
// a control character are dropped.
func LoadPreferences(path string) (Preferences, error) {
	var raw Preferences
	found, err := decodePreferences(path, &raw)
	if err != nil || !found {
		return Preferences{}, err
	}
	if raw.Version != preferencesVersion {
		return Preferences{}, fmt.Errorf("preferences version %d is not supported", raw.Version)
	}
	p := Preferences{Version: preferencesVersion}
	for verb, entries := range raw.Defaults {
		if !validVerb(verb) {
			return Preferences{}, fmt.Errorf("preferences name an unknown verb %q", verb)
		}
		for key, id := range entries {
			if validPreferenceString(key) && validPreferenceString(id) {
				p.SetDefault(verb, key, id)
			}
		}
	}
	p.Hidden = cleanHidden(raw.Hidden)
	return p, nil
}

// LoadLegacyPreferences reads gloss's old file, {"defaults":{kind:id},
// "hidden":[...]}, with the same bounds. Its defaults become Defaults[Open].
// A missing file gives empty preferences.
func LoadLegacyPreferences(path string) (Preferences, error) {
	var raw legacyPreferences
	found, err := decodePreferences(path, &raw)
	if err != nil || !found {
		return Preferences{}, err
	}
	p := Preferences{Version: preferencesVersion}
	for key, id := range raw.Defaults {
		if validPreferenceString(key) && validPreferenceString(id) {
			p.SetDefault(Open, key, id)
		}
	}
	p.Hidden = cleanHidden(raw.Hidden)
	return p, nil
}

// Save writes the preferences to path with Version 1. It writes a temporary
// file in the same folder and renames it over path, creating the folder 0700,
// so a failure leaves any earlier file intact.
func (p Preferences) Save(path string) error {
	p.Version = preferencesVersion
	for verb := range p.Defaults {
		if !validVerb(verb) {
			return fmt.Errorf("preferences name an unknown verb %q", verb)
		}
	}
	if p.Defaults == nil {
		p.Defaults = map[Verb]map[string]string{}
	}
	if p.Hidden == nil {
		p.Hidden = []string{}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(b) > maxPreferences {
		return errors.New("preferences exceed 64 KiB")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Default returns the ID chosen for verb and key (a Target's Key), or "".
func (p Preferences) Default(v Verb, key string) string {
	return p.Defaults[v][key]
}

// SetDefault chooses the app with the given ID for verb and key. An empty ID
// removes the choice.
func (p *Preferences) SetDefault(v Verb, key, id string) {
	if id == "" {
		delete(p.Defaults[v], key)
		if len(p.Defaults[v]) == 0 {
			delete(p.Defaults, v)
		}
		return
	}
	if p.Defaults == nil {
		p.Defaults = map[Verb]map[string]string{}
	}
	if p.Defaults[v] == nil {
		p.Defaults[v] = map[string]string{}
	}
	p.Defaults[v][key] = id
}

// IsHidden reports whether the app with this ID is hidden.
func (p Preferences) IsHidden(id string) bool {
	return slices.Contains(p.Hidden, id)
}

// SetHidden hides or shows the app with this ID. An ID is listed once.
func (p *Preferences) SetHidden(id string, hidden bool) {
	switch {
	case hidden && !p.IsHidden(id):
		p.Hidden = append(p.Hidden, id)
	case !hidden:
		p.Hidden = slices.DeleteFunc(p.Hidden, func(h string) bool { return h == id })
	}
}

func validVerb(v Verb) bool { return v == Open || v == Edit }

// validPreferenceString reports whether s may be a key or an ID.
func validPreferenceString(s string) bool {
	if s == "" || len(s) > maxPreferenceString {
		return false
	}
	return !slices.ContainsFunc([]rune(s), unicode.IsControl)
}

// cleanHidden drops invalid and repeated IDs.
func cleanHidden(ids []string) []string {
	var out []string
	for _, id := range ids {
		if validPreferenceString(id) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// decodePreferences reads path into v under the file rules. found is false,
// with no error, when the file does not exist.
func decodePreferences(path string, v any) (found bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxPreferences+1))
	if err != nil {
		return false, err
	}
	if len(b) > maxPreferences {
		return false, errors.New("preferences exceed 64 KiB")
	}
	if b = bytes.TrimSpace(b); len(b) == 0 || b[0] != '{' {
		return false, errors.New("preferences must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false, fmt.Errorf("preferences: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return false, errors.New("preferences have trailing data")
	}
	return true, nil
}
