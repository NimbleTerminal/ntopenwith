package winassoc

import (
	"reflect"
	"testing"
)

func TestFriendlyNameFallbacks(t *testing.T) {
	for _, tt := range []struct {
		name, queried, application, resolved, exe, description, want string
		calls                                                        []string
	}{
		{name: "shell name", queried: "Paint", want: "Paint", calls: []string{"friendly"}},
		{name: "application name", application: "Notepad", want: "Notepad", calls: []string{"friendly", "application"}},
		{name: "package resource", application: "@{package?resource}", resolved: "Notepad", want: "Notepad", calls: []string{"friendly", "application", "indirect"}},
		{name: "DLL resource", application: "@app.dll,-123", resolved: "Editor", want: "Editor", calls: []string{"friendly", "application", "indirect"}},
		{name: "file description", exe: `C:\Program Files\Editor\editor.exe`, description: "Text Editor", want: "Text Editor", calls: []string{"friendly", "application", "executable", "description"}},
		{name: "unresolved resource", application: "@missing.dll,-1", exe: `C:\Windows\NOTEPAD.EXE`, want: "NOTEPAD", calls: []string{"friendly", "application", "indirect", "executable", "description"}},
		{name: "basename", exe: `C:\Program Files\Editor\editor.exe`, want: "editor", calls: []string{"friendly", "application", "executable", "description"}},
		{name: "no name", want: "txtfile", calls: []string{"friendly", "application", "executable"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			look := nameLookups{
				query: func(class, verb string, kind uint32) string {
					if class != "txtfile" || verb != "open" {
						t.Fatalf("query(%q, %q)", class, verb)
					}
					switch kind {
					case assocstrFriendlyAppName:
						calls = append(calls, "friendly")
						return tt.queried
					case assocstrExecutable:
						calls = append(calls, "executable")
						return tt.exe
					default:
						t.Fatalf("unexpected query %d", kind)
						return ""
					}
				},
				application: func(class string) string {
					if class != "txtfile" {
						t.Fatalf("application(%q)", class)
					}
					calls = append(calls, "application")
					return tt.application
				},
				indirect: func(s string) string {
					if s != tt.application {
						t.Fatalf("indirect(%q)", s)
					}
					calls = append(calls, "indirect")
					return tt.resolved
				},
				description: func(path string) string {
					if path != tt.exe {
						t.Fatalf("description(%q)", path)
					}
					calls = append(calls, "description")
					return tt.description
				},
			}
			if got := friendlyName("txtfile", "open", look); got != tt.want {
				t.Errorf("name = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(calls, tt.calls) {
				t.Errorf("calls = %v, want %v", calls, tt.calls)
			}
		})
	}
}
