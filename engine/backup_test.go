// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func backupFixture(t *testing.T) (*testNetwork, *BackupSender, string) {
	t.Helper()
	n := newNetwork(t, true)
	folder(n.a).cfg.ReceiveBackups = true
	n.cfgA.Folders[0].ReceiveBackups = true
	n.poll(t)
	cfg := n.cfgA
	cfg.Folders = append([]FolderConfig(nil), cfg.Folders...)
	cfg.Client.DeviceID = "phone"
	cfg.Folders[0].Path = t.TempDir()
	sender, err := NewBackupSender(cfg, cfg.Folders[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return n, sender, cfg.Folders[0].Path
}

func TestCameraBackupResumesVerifiedChunksAfterReceiverRestart(t *testing.T) {
	n, sender, phone := backupFixture(t)
	body := strings.Repeat("video-content", (3<<20)/13+1)
	source := filepath.Join(phone, "video")
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := n.a.server.Handler
	n.a.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original.ServeHTTP(w, r)
		if r.Method == http.MethodPut {
			cancel()
		}
	})
	if _, err := sender.Upload(ctx, source, "video.mp4", time.Time{}); err == nil {
		t.Fatal("interrupted video was acknowledged")
	}
	if len(folder(n.a).snapshot()) != 0 {
		t.Fatal("partial video entered the shared manifest")
	}
	dir, err := folder(n.a).root.Open(".douchesync/incoming")
	if err != nil {
		t.Fatal(err)
	}
	parts, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil || len(parts) != 1 {
		t.Fatalf("missing partial: %v %v", parts, err)
	}
	info, err := folder(n.a).root.Stat(".douchesync/incoming/" + parts[0])
	if err != nil || info.Size() != backupChunkSize {
		t.Fatalf("wrong checkpoint: %v %v", info, err)
	}
	n.a.Close()
	n.a, err = NewClient(n.cfgA)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var offsets []int64
	original = n.a.server.Handler
	n.a.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			v, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
			mu.Lock()
			offsets = append(offsets, v)
			mu.Unlock()
		}
		original.ServeHTTP(w, r)
	})
	receipt, err := sender.Upload(context.Background(), source, "video.mp4", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(offsets) == 0 || offsets[0] != backupChunkSize {
		t.Fatalf("video restarted instead of resuming: %v", offsets)
	}
	got, err := os.ReadFile(filepath.Join(n.dirA, filepath.FromSlash(receipt.Path)))
	if err != nil || string(got) != body {
		t.Fatal("resumed video failed byte verification")
	}
}

func TestCorruptPartialCannotProduceACompletedReceipt(t *testing.T) {
	n, sender, phone := backupFixture(t)
	body := strings.Repeat("v", backupChunkSize+17)
	source := filepath.Join(phone, "video")
	os.WriteFile(source, []byte(body), 0600)
	original := n.a.server.Handler
	n.a.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original.ServeHTTP(w, r)
		if r.Method == http.MethodPut && r.URL.Query().Get("offset") == "0" {
			p, _ := backupPath("phone", "video.mp4", digest([]byte(body)))
			key := digest([]byte(p))
			folder(n.a).root.WriteFile(".douchesync/incoming/"+key+".part", []byte(strings.Repeat("x", backupChunkSize)), 0600)
		}
	})
	if _, err := sender.Upload(context.Background(), source, "video.mp4", time.Time{}); err == nil {
		t.Fatal("a corrupt prefix was acknowledged")
	}
	if len(folder(n.a).snapshot()) != 0 {
		t.Fatal("corrupt partial was published")
	}
	n.a.server.Handler = original
	if _, err := sender.Upload(context.Background(), source, "video.mp4", time.Time{}); err != nil {
		t.Fatal("clean retry could not recover:", err)
	}
}

func TestCameraBackupIsDirectIdempotentAndNeverRestoresPhoneFiles(t *testing.T) {
	n, sender, phone := backupFixture(t)
	source := filepath.Join(phone, "photo.jpg")
	if err := os.WriteFile(source, []byte("camera photo"), 0600); err != nil {
		t.Fatal(err)
	}
	receipt, err := sender.Upload(context.Background(), source, "IMG_001.jpg", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Peer != "alpha" || receipt.Hash != digest([]byte("camera photo")) {
		t.Fatalf("bad receipt: %+v", receipt)
	}
	data, err := os.ReadFile(filepath.Join(n.dirA, filepath.FromSlash(receipt.Path)))
	if err != nil || string(data) != "camera photo" {
		t.Fatalf("bad backup: %q %v", data, err)
	}
	before := folder(n.a).snapshot()[receipt.Path]
	second, err := sender.Upload(context.Background(), source, "IMG_001.jpg", time.Now().Add(-time.Hour))
	if err != nil || second.Path != receipt.Path {
		t.Fatalf("retry failed: %+v %v", second, err)
	}
	if compareClock(before.Clock, folder(n.a).snapshot()[receipt.Path].Clock) != 0 {
		t.Fatal("retry created a new version")
	}
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	n.cycle(t)
	for _, dir := range []string{n.dirA, n.dirB} {
		if _, err = os.Stat(filepath.Join(dir, filepath.FromSlash(receipt.Path))); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(phone)
	if err != nil || len(entries) != 0 {
		t.Fatal("phone deletion was restored or phone listener created private history")
	}
}

func TestCameraFilenameCollisionsAndEditedBackupsArePreserved(t *testing.T) {
	n, sender, phone := backupFixture(t)
	source := filepath.Join(phone, "photo")
	os.WriteFile(source, []byte("first"), 0600)
	a, err := sender.Upload(context.Background(), source, "IMG_001.jpg", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(source, []byte("second"), 0600)
	b, err := sender.Upload(context.Background(), source, "IMG_001.jpg", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Path == b.Path {
		t.Fatal("different photos with the same name collided")
	}
	put(t, n.dirA, a.Path, "local edit")
	os.WriteFile(source, []byte("first"), 0600)
	if _, err = sender.Upload(context.Background(), source, "IMG_001.jpg", time.Time{}); err == nil {
		t.Fatal("edited backup was overwritten")
	}
	data, _ := os.ReadFile(filepath.Join(n.dirA, filepath.FromSlash(a.Path)))
	if string(data) != "local edit" {
		t.Fatal("local edit was lost")
	}
}

func TestBackupHistoryFailureIsNotAcknowledgedAndRetryRecovers(t *testing.T) {
	n, sender, phone := backupFixture(t)
	f := folder(n.a)
	if err := f.root.Rename(".douchesync/updates", ".douchesync/updates-saved"); err != nil {
		t.Fatal(err)
	}
	if err := f.root.WriteFile(".douchesync/updates", []byte("obstruction"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(phone, "photo")
	os.WriteFile(source, []byte("durable backup"), 0600)
	if _, err := sender.Upload(context.Background(), source, "photo.jpg", time.Time{}); err == nil {
		t.Fatal("failed history was acknowledged")
	}
	if _, err := f.manifest(); err == nil {
		t.Fatal("unhealthy backup history was served")
	}
	f.root.Remove(".douchesync/updates")
	f.root.Rename(".douchesync/updates-saved", ".douchesync/updates")
	if err := f.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Upload(context.Background(), source, "photo.jpg", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if len(f.snapshot()) != 1 {
		t.Fatal("retry duplicated the unacknowledged photo")
	}
}

func TestBackupHistorySurvivesRestart(t *testing.T) {
	n, sender, phone := backupFixture(t)
	source := filepath.Join(phone, "photo")
	os.WriteFile(source, []byte("restart"), 0600)
	r, err := sender.Upload(context.Background(), source, "photo.jpg", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	before := folder(n.a).snapshot()[r.Path]
	n.a.Close()
	n.a, err = NewClient(n.cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if got := folder(n.a).snapshot()[r.Path]; got.Hash != before.Hash || compareClock(got.Clock, before.Clock) != 0 {
		t.Fatal("pending backup receipt was not recovered")
	}
}

func TestBackupRejectsBadAuthenticationCorruptionAndTraversal(t *testing.T) {
	n, _, _ := backupFixture(t)
	peer := Announcement{URL: n.a.localURL, Fingerprint: n.a.fingerprint}
	client := pinnedClient(peer, 5*time.Second)
	defer client.CloseIdleConnections()
	for _, test := range []struct {
		name, hash string
		sign       bool
		status     int
	}{
		{"photo.jpg", digest([]byte("body")), false, 401},
		{"photo.jpg", strings.Repeat("0", 64), true, 400},
		{"../../escape.jpg", digest([]byte("body")), true, 400},
	} {
		q := url.Values{"room": {roomID(folder(n.a).cfg)}, "device": {"phone"}, "name": {test.name}, "hash": {test.hash}, "size": {strconv.Itoa(4)}, "modified": {"0"}}
		r, _ := http.NewRequest(http.MethodPut, peer.URL+"/v1/backup?"+q.Encode(), strings.NewReader("body"))
		if test.sign {
			signRequest(r, testSecret)
		}
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != test.status {
			t.Fatalf("%s: got %d want %d", test.name, resp.StatusCode, test.status)
		}
	}
	if len(folder(n.a).snapshot()) != 0 {
		t.Fatal("invalid request mutated history")
	}
	files, _ := os.ReadDir(filepath.Join(n.dirA, ".douchesync/transfers"))
	if len(files) != 0 {
		t.Fatal("invalid upload left temporary files")
	}
}

func TestBackupRequiresExplicitDesktopOptInAndHonorsCancellation(t *testing.T) {
	n, sender, phone := backupFixture(t)
	folder(n.a).cfg.ReceiveBackups = false
	source := filepath.Join(phone, "photo")
	os.WriteFile(source, []byte("photo"), 0600)
	if _, err := sender.Upload(context.Background(), source, "photo.jpg", time.Time{}); err == nil {
		t.Fatal("backup receiver was enabled by default")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sender.Upload(ctx, source, "photo.jpg", time.Time{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not propagated: %v", err)
	}
}
