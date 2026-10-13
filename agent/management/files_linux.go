package management

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func privileged() bool { return os.Geteuid() == 0 }
func openNonblocking(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
func regularSingleLink(info os.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}

func secureRoot(name string) error {
	for {
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 {
			return errors.New("unprotected root")
		}
		parent := filepath.Dir(name)
		if parent == name {
			return nil
		}
		name = parent
	}
}

func fileWritable(name string) bool { return secureRoot(name) == nil && syscall.Access(name, 2) == nil }
