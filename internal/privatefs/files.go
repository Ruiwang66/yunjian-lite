// Package privatefs prevents secret files from inheriting permissive ACLs or
// being written through symlinks. It is used only on explicitly configured paths.
package privatefs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func Directory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private directory must be a real directory")
	}
	return protect(path, true)
}
func File(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("private file must be a regular file")
	}
	return protect(path, false)
}
func AtomicWrite(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace a non-regular secret file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = protect(f.Name(), false); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func ReadSecret(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 512 {
		return nil, fmt.Errorf("secret must be a small regular file")
	}
	if err = readableOnlyByOwners(path, info); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, fmt.Errorf("secret file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, 513))
	if len(data) > 512 {
		return nil, fmt.Errorf("secret file is too large")
	}
	return data, err
}
