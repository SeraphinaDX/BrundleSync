// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"log"
	"os"
	"path"
	"slices"
)

var errDirectoryNotEmpty = errors.New("directory is not empty")

// Only ancestors of deleted tracked files are eligible. No recursive directory
// walk is needed, and unrelated empty folders and the shared root are preserved.
// Call with f.mu held, just like the file/history commit.
func (f *Folder) queueDeletedParentsLocked(p string) {
	if !f.cfg.SyncDeletes || validatePath(p) != nil || f.ignored(p) {
		return
	}
	for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
		if f.deleteParents == nil {
			f.deleteParents = make(map[string]struct{})
		}
		f.deleteParents[dir] = struct{}{}
	}
}

// Run once per deletion batch, after its checkpoint, or after the startup scan.
// Deepest-first deduplication handles parallel deletions without repeated walks
// of a large tree. Cancellation retains unattempted work for the next cycle.
func (f *Folder) pruneDeletedParentsLocked(ctx context.Context) {
	if len(f.deleteParents) == 0 || !f.cfg.SyncDeletes || !f.healthy || f.dirty || f.pendingUpdates != 0 {
		return
	}
	dirs := make([]string, 0, len(f.deleteParents))
	for dir := range f.deleteParents {
		dirs = append(dirs, dir)
	}
	slices.SortFunc(dirs, func(a, b string) int { return len(b) - len(a) })
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return
		}
		delete(f.deleteParents, dir)
		if f.ignored(dir) {
			continue
		}
		err := f.checkAncestors(dir)
		if err == nil {
			// Directory-only OS operations refuse files, symlinks and nonempty
			// directories, even if another process changes them during cleanup.
			err = removeEmptyDirectory(f.root, dir)
		}
		if err == nil {
			log.Printf("[%s] removed empty directory %s", f.cfg.ID, dir)
		} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errDirectoryNotEmpty) {
			log.Printf("[%s] keeping directory %s: %v", f.cfg.ID, dir, err)
		}
	}
	f.deleteParents = nil
}
