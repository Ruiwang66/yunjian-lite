//go:build !windows

package privatefs

import (
	"fmt"
	"os"
)

func protect(path string, dir bool) error {
	mode := os.FileMode(0600)
	if dir {
		mode = 0700
	}
	return os.Chmod(path, mode)
}
func readableOnlyByOwners(_ string, info os.FileInfo) error {
	// A read-only service group (0640) is supported; world access and group writes are not.
	if info.Mode().Perm()&0027 != 0 {
		return fmt.Errorf("secret file must deny world access and group writes")
	}
	return nil
}
