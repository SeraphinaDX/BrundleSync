// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"douchesync/engine"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func desktopConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared")
	if err := os.Mkdir(shared, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := engine.Config{Client: engine.ClientConfig{DeviceID: "desktop-test", IdentityDir: filepath.Join(dir, "identity"), Listen: "127.0.0.1:0", DiscoveryURL: "https://127.0.0.1:1", DiscoveryToken: engine.GenerateSecret()}, Folders: []engine.FolderConfig{{ID: "test", Path: shared, Secret: engine.GenerateSecret()}}}
	text, err := engine.EncodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "config.toml")
	if err = engine.SaveConfigText(filename, text); err != nil {
		t.Fatal(err)
	}
	return filename
}
func TestControllerStopWaitsForIdentityAndFolderLocks(t *testing.T) {
	filename := desktopConfig(t)
	c := &controller{}
	if err := c.start(filename); err != nil {
		t.Fatal(err)
	}
	if err := c.start(filename); err == nil {
		t.Fatal("second engine was started")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s, _ := c.snapshot()
		if s.Device != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("engine did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-c.stop():
	case <-time.After(8 * time.Second):
		t.Fatal("engine did not stop")
	}
	cfg, err := engine.LoadConfig(filename)
	if err != nil {
		t.Fatal(err)
	}
	client, err := engine.NewClient(cfg)
	if err != nil {
		t.Fatalf("controller retained locks: %v", err)
	}
	client.Close()
	if _, running := c.snapshot(); running {
		t.Fatal("controller still reports running")
	}
}
func TestInvalidGUIConfigDoesNotStartOrReplaceWorkingConfig(t *testing.T) {
	filename := desktopConfig(t)
	before, _ := os.ReadFile(filename)
	if err := engine.SaveConfigText(filename, "[client]\ndevice_id='broken'\n"); err == nil {
		t.Fatal("invalid configuration was saved")
	}
	after, _ := os.ReadFile(filename)
	if string(before) != string(after) {
		t.Fatal("working configuration was changed")
	}
}
