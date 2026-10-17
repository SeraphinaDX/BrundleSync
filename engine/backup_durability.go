// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"errors"
	"path"
	"path/filepath"
	"runtime"
	"syscall"
)

func (f *Folder) syncDirectory(p string) error {
	// Windows doesn't expose a flushable directory handle through os.Root;
	// its file data and journal records are flushed with File.Sync instead.
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := f.root.Open(filepath.FromSlash(p))
	if err != nil {
		return err
	}
	defer d.Close()
	err = d.Sync()
	if errors.Is(err, syscall.EINVAL) {
		return nil
	} // Some filesystems do not implement directory fsync.
	return err
}

func (f *Folder) syncBackupDirectories(p string) error {
	for dir := path.Dir(p); ; dir = path.Dir(dir) {
		if err := f.syncDirectory(dir); err != nil {
			return err
		}
		if dir == "." {
			break
		}
	}
	if err := f.syncDirectory(".douchesync/updates"); err != nil {
		return err
	}
	return f.syncDirectory(".douchesync")
}
