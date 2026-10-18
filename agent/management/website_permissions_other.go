//go:build !linux

package management

import "os"

func rootOwned(info os.FileInfo) bool { return true }
func mPathLinux() bool                { return false }
