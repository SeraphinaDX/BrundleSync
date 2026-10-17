// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestCancelledScanLeavesCheckpointIntactAndCanRecover(t *testing.T) {
	f := cacheFixture(t, 2, 1024)
	before, err := f.root.ReadFile(".douchesync/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.root.WriteFile("new-photo", []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = f.ScanContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after, err := f.root.ReadFile(".douchesync/state.json")
	if err != nil || string(after) != string(before) {
		t.Fatal("cancelled scan changed its durable checkpoint")
	}
	if _, err = f.manifest(); err == nil {
		t.Fatal("interrupted scan published a partial manifest")
	}
	if err = f.Scan(); err != nil {
		t.Fatal(err)
	}
	if f.snapshot()["new-photo"].Hash == "" {
		t.Fatal("recovery scan lost the new file")
	}
	if _, err = os.Stat(f.cfg.Path); err != nil {
		t.Fatal(err)
	}
}

func TestAppendFoldersPreservesCommentsAndAndroidSettings(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	text, err := EncodeConfig(Config{Client: ClientConfig{DeviceID: "test", Listen: "127.0.0.1:0", DiscoveryURL: "https://sync.example.com", DiscoveryToken: testToken}, Folders: []FolderConfig{{ID: "first", Path: a, Secret: testSecret}}, Android: AndroidConfig{CameraURI: "content://camera/tree/root"}})
	if err != nil {
		t.Fatal(err)
	}
	text = "# Keep this comment\n" + text
	for i, dir := range []string{b, c} {
		text, err = AppendFolderConfig(text, FolderConfig{ID: []string{"second", "third"}[i], Path: dir, Secret: testSecret})
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := DecodeConfig(text)
	if err != nil || len(cfg.Folders) != 3 || cfg.Android.CameraURI != "content://camera/tree/root" {
		t.Fatalf("bad append: %+v %v", cfg, err)
	}
	if text[:19] != "# Keep this comment" {
		t.Fatal("comment was changed")
	}
}

func TestIncomingRequestLockCancellationDoesNotLeakOrUnlockAnotherRequest(t *testing.T) {
	f := historyFixture(t)
	release, err := f.lockIncoming(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = f.lockIncoming(ctx, "key"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	done := make(chan struct{})
	go func() {
		release, err := f.lockIncoming(context.Background(), "key")
		if err == nil {
			close(entered)
			release()
		}
		close(done)
	}()
	select {
	case <-entered:
		t.Fatal("cancelled waiter released another request's lock")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request lock was never released")
	}
	f.incomingMu.Lock()
	defer f.incomingMu.Unlock()
	if len(f.incomingLocks) != 0 {
		t.Fatal("request locks leaked")
	}
}
