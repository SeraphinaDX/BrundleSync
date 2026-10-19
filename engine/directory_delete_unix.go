// SPDX-License-Identifier: GPL-3.0-or-later
//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package engine

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func removeEmptyDirectory(root *os.Root, name string) error {
	if err := validatePath(name); err != nil {
		return err
	}
	base, err := root.Open(".")
	if err != nil {
		return err
	}
	defer base.Close()
	fd := int(base.Fd())
	defer func() {
		if fd != int(base.Fd()) {
			unix.Close(fd)
		}
	}()
	parts := strings.Split(name, "/")
	// Resolve each ancestor relative to an already-open directory, refusing
	// symlinks atomically. Never fall back to an unrooted filesystem path.
	for _, part := range parts[:len(parts)-1] {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		if fd != int(base.Fd()) {
			unix.Close(fd)
		}
		fd = next
	}
	err = unix.Unlinkat(fd, parts[len(parts)-1], unix.AT_REMOVEDIR)
	if errors.Is(err, unix.ENOTEMPTY) || errors.Is(err, unix.EEXIST) {
		return errDirectoryNotEmpty
	}
	return err
}
