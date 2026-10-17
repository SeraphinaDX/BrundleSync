// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

type incomingLock struct {
	token      chan struct{}
	references int
}

// Locks are reference counted and removed after each request, so an unbounded
// sequence of camera filenames cannot leave an unbounded mutex registry.
func (f *Folder) lockIncoming(ctx context.Context, key string) (func(), error) {
	f.incomingMu.Lock()
	if f.incomingLocks == nil {
		f.incomingLocks = map[string]*incomingLock{}
	}
	lock := f.incomingLocks[key]
	if lock == nil {
		lock = &incomingLock{token: make(chan struct{}, 1)}
		f.incomingLocks[key] = lock
	}
	lock.references++
	f.incomingMu.Unlock()
	unref := func() {
		f.incomingMu.Lock()
		defer f.incomingMu.Unlock()
		lock.references--
		if lock.references == 0 {
			delete(f.incomingLocks, key)
		}
	}
	select {
	case lock.token <- struct{}{}:
		return func() { <-lock.token; unref() }, nil
	case <-ctx.Done():
		unref()
		return nil, ctx.Err()
	}
}

func (f *Folder) openIncoming(part string) (*os.File, error) {
	f.incomingMu.Lock()
	defer f.incomingMu.Unlock()
	if info, err := f.root.Lstat(part); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("incomplete backup is not a regular file")
		}
		return f.root.OpenFile(part, os.O_RDWR, 0600)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := f.cleanupIncomingLocked(); err != nil {
		return nil, err
	}
	d, err := f.root.Open(".douchesync/incoming")
	if err != nil {
		return nil, err
	}
	names, err := d.Readdirnames(65)
	d.Close()
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(names) >= 64 {
		return nil, errors.New("too many unfinished backups")
	}
	return f.root.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
}

// An interrupted upload remains resumable for seven days. Collection runs at
// startup and when creating a new partial file, never while that key is active.
func (f *Folder) cleanupIncomingLocked() error {
	d, err := f.root.Open(".douchesync/incoming")
	if err != nil {
		return err
	}
	names, err := d.Readdirnames(maxEntries + 1)
	d.Close()
	if err != nil && err != io.EOF {
		return err
	}
	if len(names) > maxEntries {
		return errors.New("too many incoming backup files")
	}
	for _, name := range names {
		if !strings.HasSuffix(name, ".part") {
			continue
		}
		if f.incomingLocks[strings.TrimSuffix(name, ".part")] != nil {
			continue
		}
		p := ".douchesync/incoming/" + name
		info, err := f.root.Lstat(p)
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && time.Since(info.ModTime()) > 7*24*time.Hour {
			if err = f.root.Remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}
