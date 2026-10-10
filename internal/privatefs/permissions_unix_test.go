//go:build !windows

package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnixPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := Directory(dir); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal("directory not private")
	}
	path := filepath.Join(dir, "key")
	if err := AtomicWrite(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("secret not private")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(path); err == nil {
		t.Fatal("world-readable key accepted")
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(path); err != nil {
		t.Fatal("service group key refused", err)
	}
}
