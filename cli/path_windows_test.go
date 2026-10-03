package cli

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// On a key of its own, never the real Environment.
func TestRegistryPath(t *testing.T) {
	key := fmt.Sprintf(`Software\DAWGit-test-%d`, time.Now().UnixNano())
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.ALL_ACCESS)
	if err != nil {
		t.Skip(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, key)
	k.SetExpandStringValue("Path", `%SystemRoot%\system32;C:\tools`)
	k.Close()
	bin := `C:\Users\yi\AppData\Local\Programs\DAWGit\bin`
	read := func() (string, uint32) {
		k, _ := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
		defer k.Close()
		v, kind, _ := k.GetStringValue("Path")
		return v, kind
	}
	if ch, err := editRegistryPath(key, true, bin); err != nil || !ch {
		t.Fatal(ch, err)
	}
	if ch, _ := editRegistryPath(key, true, bin); ch {
		t.Error("added twice")
	}
	if v, kind := read(); v != `%SystemRoot%\system32;C:\tools;`+bin || kind != registry.EXPAND_SZ {
		t.Fatalf("after add: %q (kind %d)", v, kind)
	}
	if ch, err := editRegistryPath(key, false, bin); err != nil || !ch {
		t.Fatal(ch, err)
	}
	if v, _ := read(); v != `%SystemRoot%\system32;C:\tools` {
		t.Fatalf("after remove: %q", v)
	}
}
