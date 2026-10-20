package winassoc

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// OS is the Windows registry, read-only.
func OS() Registry { return osRegistry{} }

type osRegistry struct{}

func open(root, path string, access uint32) (registry.Key, error) {
	var k registry.Key
	switch root {
	case "HKCU":
		k = registry.CURRENT_USER
	case "HKLM":
		k = registry.LOCAL_MACHINE
	case "HKCR":
		k = registry.CLASSES_ROOT
	default:
		return 0, fmt.Errorf("unknown registry root %q", root)
	}
	return registry.OpenKey(k, path, access)
}

func (osRegistry) String(root, path, name string) (string, error) {
	k, err := open(root, path, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	s, _, err := k.GetStringValue(name)
	return s, err
}

func (osRegistry) ValueNames(root, path string) ([]string, error) {
	k, err := open(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	return k.ReadValueNames(-1)
}

func (osRegistry) SubKeys(root, path string) ([]string, error) {
	k, err := open(root, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	return k.ReadSubKeyNames(-1)
}
