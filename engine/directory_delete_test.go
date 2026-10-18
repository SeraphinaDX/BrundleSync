// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncedDirectoryDeletionRemovesEmptyParents(t *testing.T) {
	n := newNetwork(t, true)
	for i := range 12 {
		put(t, n.dirA, fmt.Sprintf("Code/project-%d/src/file.go", i), "retained source")
	}
	n.poll(t)
	n.cycle(t)
	if err := os.Mkdir(filepath.Join(n.dirB, "unrelated-empty"), 0755); err != nil {
		t.Fatal(err)
	}
	originals := folder(n.b).snapshot()
	if err := os.RemoveAll(filepath.Join(n.dirA, "Code")); err != nil {
		t.Fatal(err)
	}
	n.cycle(t)
	if _, err := os.Stat(filepath.Join(n.dirB, "Code")); !os.IsNotExist(err) {
		t.Fatalf("deleted tree left empty directories behind: %v", err)
	}
	for _, dir := range []string{n.dirB, filepath.Join(n.dirB, "unrelated-empty"), filepath.Join(n.dirB, ".douchesync")} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("unrelated directory removed: %s (%v)", dir, err)
		}
	}
	for p, old := range originals {
		if !folder(n.b).snapshot()[p].Deleted {
			t.Fatalf("missing deletion history for %s", p)
		}
		b, err := folder(n.b).root.ReadFile(archiveName("versions", p, old))
		if err != nil || string(b) != "retained source" {
			t.Fatalf("recovery copy lost for %s: %v", p, err)
		}
	}
	if err := n.b.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryCleanupPreservesRemainingContents(t *testing.T) {
	for _, keep := range []string{"local.txt", "ignored.tmp", "unrelated-empty"} {
		t.Run(keep, func(t *testing.T) {
			n := newNetwork(t, true)
			folder(n.b).cfg.Ignore = []string{"*.tmp"}
			put(t, n.dirA, "Code/src/gone.go", "source")
			n.poll(t)
			n.cycle(t)
			if keep == "unrelated-empty" {
				if err := os.Mkdir(filepath.Join(n.dirB, "Code", keep), 0755); err != nil {
					t.Fatal(err)
				}
			} else {
				put(t, n.dirB, "Code/"+keep, "keep me")
			}
			if err := os.RemoveAll(filepath.Join(n.dirA, "Code")); err != nil {
				t.Fatal(err)
			}
			n.cycle(t)
			if _, err := os.Stat(filepath.Join(n.dirB, "Code", keep)); err != nil {
				t.Fatalf("remaining local content removed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(n.dirB, "Code", "src")); !os.IsNotExist(err) {
				t.Fatalf("empty deleted branch retained: %v", err)
			}
		})
	}
}

func TestDirectoryCleanupIsDurableResumableAndOneShot(t *testing.T) {
	f := historyFixture(t)
	if err := f.root.MkdirAll("Code/src", 0755); err != nil {
		t.Fatal(err)
	}
	if err := f.root.WriteFile("Code/src/gone.go", []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := f.Scan(); err != nil {
		t.Fatal(err)
	}
	p := "Code/src/gone.go"
	if err := (&Client{}).applyDeletion(context.Background(), Announcement{}, f, p, tombstone(f.state.Entries[p])); err != nil {
		t.Fatal(err)
	}
	f.pruneDeletedParentsLocked(context.Background())
	if _, err := f.root.Stat("Code/src"); err != nil {
		t.Fatal("parent removed before deletion checkpoint")
	}
	if err := f.saveLocked(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.pruneDeletedParentsLocked(ctx)
	if len(f.deleteParents) == 0 {
		t.Fatal("cancelled cleanup lost pending directories")
	}
	// Simulate an older version or a crash after the file deletion checkpoint.
	f.Close()
	reopened, err := openFolder(f.cfg, f.device)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.root.Stat("Code"); !os.IsNotExist(err) {
		t.Fatalf("startup did not repair empty parents: %v", err)
	}
	if len(reopened.deleteParents) != 0 {
		t.Fatal("completed cleanup retained idle work")
	}
	if err := reopened.root.Mkdir("Code", 0755); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.root.Stat("Code"); err != nil {
		t.Fatal("old tombstone repeatedly removed a newly created empty directory")
	}
}

func TestDirectoryCleanupRespectsDeletionOptOutAndIgnores(t *testing.T) {
	for _, kind := range []string{"disabled", "ignored"} {
		t.Run(kind, func(t *testing.T) {
			f := historyFixture(t)
			if err := f.root.MkdirAll("Code/src", 0755); err != nil {
				t.Fatal(err)
			}
			f.state.Entries["Code/src/gone.go"] = Entry{Deleted: true, Clock: Clock{"sender": 2}}
			if err := f.saveLocked(); err != nil {
				t.Fatal(err)
			}
			cfg := f.cfg
			if kind == "disabled" {
				cfg.SyncDeletes = false
			} else {
				cfg.Ignore = []string{"Code"}
			}
			f.Close()
			reopened, err := openFolder(cfg, f.device)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if err := reopened.Scan(); err != nil {
				t.Fatal(err)
			}
			if _, err := reopened.root.Stat("Code/src"); err != nil {
				t.Fatalf("protected directory removed: %v", err)
			}
		})
	}
}

func TestDirectoryRemovalRefusesFilesLinksAndEscapes(t *testing.T) {
	f := historyFixture(t)
	if err := f.root.WriteFile("file", []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := f.root.MkdirAll("nonempty/child", 0755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"file", "nonempty", "..", ".douchesync"} {
		if err := removeEmptyDirectory(f.root, p); err == nil {
			t.Fatalf("removed protected path: %s", p)
		}
	}
	if err := removeEmptyDirectory(f.root, "nonempty"); !errors.Is(err, errDirectoryNotEmpty) {
		t.Fatalf("unexpected nonempty error: %v", err)
	}
	if b, err := f.root.ReadFile("file"); err != nil || string(b) != "keep" {
		t.Fatal("directory-only removal deleted a file")
	}
	if err := f.root.Mkdir("empty", 0755); err != nil {
		t.Fatal(err)
	}
	if err := removeEmptyDirectory(f.root, "empty"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "child"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.cfg.Path, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, p := range []string{"link", "link/child"} {
		if err := removeEmptyDirectory(f.root, p); err == nil {
			t.Fatalf("followed or removed a symlink: %s", p)
		}
	}
	if _, err := os.Lstat(filepath.Join(f.cfg.Path, "link")); err != nil {
		t.Fatal("symlink removed")
	}
	if _, err := os.Stat(filepath.Join(outside, "child")); err != nil {
		t.Fatal("directory outside root removed")
	}
}

func TestDirectoryCleanupWithActiveWatcher(t *testing.T) {
	f := historyFixture(t)
	if err := f.root.MkdirAll("Code/space & 日本語/src", 0755); err != nil {
		t.Fatal(err)
	}
	w, err := newFolderWatcher(f, make(chan struct{}, 1), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	f.queueDeletedParentsLocked("Code/space & 日本語/src/gone.go")
	f.pruneDeletedParentsLocked(context.Background())
	if _, err := f.root.Stat("Code"); !os.IsNotExist(err) {
		t.Fatalf("active watcher prevented empty-parent cleanup: %v", err)
	}
}

func TestDirectoryDeletionKeepsConcurrentEdit(t *testing.T) {
	n := newNetwork(t, true)
	put(t, n.dirA, "Code/src/draft.go", "base")
	n.poll(t)
	n.cycle(t)
	if err := os.RemoveAll(filepath.Join(n.dirA, "Code")); err != nil {
		t.Fatal(err)
	}
	put(t, n.dirB, "Code/src/draft.go", "offline laptop edit")
	for _, c := range []*Client{n.a, n.b} {
		if err := folder(c).Scan(); err != nil {
			t.Fatal(err)
		}
	}
	n.cycle(t)
	for _, dir := range []string{n.dirA, n.dirB} {
		if get(t, dir, "Code/src/draft.go") != "offline laptop edit" {
			t.Fatal("directory cleanup lost a concurrent edit")
		}
	}
}
