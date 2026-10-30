//go:build !linux

package security

import "os"

func openLog(name string) (*os.File, error) {
	info, e := os.Lstat(name)
	if e != nil {
		return nil, e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, os.ErrPermission
	}
	return os.Open(name)
}
