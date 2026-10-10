package privatefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateSecretLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := Directory(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "key")
	if err := AtomicWrite(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	data, err := ReadSecret(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("read: %q %v", data, err)
	}
	if err := AtomicWrite(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	data, err = ReadSecret(path)
	if err != nil || string(data) != "second" {
		t.Fatalf("replace: %q %v", data, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("secret temporary file leaked")
	}
	if err := AtomicWrite(path, []byte(strings.Repeat("x", 513))); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(path); err == nil {
		t.Fatal("oversize secret read")
	}
	if _, err := ReadSecret(dir); err == nil {
		t.Fatal("directory accepted as secret")
	}
	if err := AtomicWrite(dir, []byte("data")); err == nil {
		t.Fatal("directory overwritten")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err == nil {
		if err := AtomicWrite(link, []byte("bad")); err == nil {
			t.Fatal("symlink overwritten")
		}
		if _, err := ReadSecret(link); err == nil {
			t.Fatal("symlink read")
		}
	}
}
