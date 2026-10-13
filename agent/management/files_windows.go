package management

import (
	"golang.org/x/sys/windows"
	"os"
)

func privileged() bool                                             { return windows.GetCurrentProcessToken().IsElevated() }
func openNonblocking(root *os.Root, path string) (*os.File, error) { return root.Open(path) }
func regularSingleLink(info os.FileInfo) bool                      { return info.Mode().IsRegular() }
func secureRoot(name string) error                                 { return nil }
func fileWritable(name string) bool                                { return false }
