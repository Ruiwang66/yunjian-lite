package management

import (
	"errors"
	"os"
	"strings"
)

// Pin each parent directory independently. Lstat rejects aliases, while the
// opened-handle identity check prevents a symlink swap between check and open.
func pinDirectory(parent *os.Root, name string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("directory alias denied")
	}
	next, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	anchor, err := next.Open(".")
	if err != nil {
		next.Close()
		return nil, err
	}
	actual, err := anchor.Stat()
	anchor.Close()
	if err != nil || !os.SameFile(before, actual) {
		next.Close()
		return nil, errors.New("directory changed")
	}
	return next, nil
}
func pinParent(root *os.Root, name string) (*os.Root, string, func(), error) {
	parts := strings.Split(name, "/")
	current := root
	held := []*os.Root{}
	cleanup := func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Close()
		}
	}
	for _, part := range parts[:len(parts)-1] {
		next, err := pinDirectory(current, part)
		if err != nil {
			cleanup()
			return nil, "", func() {}, err
		}
		held = append(held, next)
		current = next
	}
	return current, parts[len(parts)-1], cleanup, nil
}
