// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"strings"
	"testing"
)

type fakeBackend struct{}

func (fakeBackend) Request(string, any, any) error { return nil }
func (fakeBackend) Launch(string) error            { return nil }
func TestReadinessAndUnsavedChanges(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	u := newPhoneUI(a, fakeBackend{})
	if u.headline.Text != "Setup needed" || !u.backup.Disabled() {
		t.Fatal("new install looked configured")
	}
	cfg := settings{DeviceID: "phone", DiscoveryURL: "https://sync.example", FolderID: "camera", CameraURI: "content://camera/tree/root"}
	u.populate(cfg)
	u.saved = snapshot{Configured: true, Settings: &cfg, Ready: true, State: "Ready"}
	u.render()
	if u.headline.Text != "✓ Configured" || u.backup.Disabled() {
		t.Fatal("saved setup did not enable backups")
	}
	u.folder.SetText("changed")
	if !u.backup.Disabled() || !strings.Contains(u.edits.Text, "Unsaved") {
		t.Fatal("unsaved settings allowed backup")
	}
	u.dirty = false
	u.saved.Ready = false
	u.render()
	if !strings.Contains(u.headline.Text, "permission") || !u.backup.Disabled() {
		t.Fatal("missing camera grant was hidden")
	}
}
func TestQRReviewPreservesPhoneSourceAndSchedule(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	u := newPhoneUI(a, fakeBackend{})
	u.populate(settings{DeviceID: "my-phone", CameraURI: "content://camera/tree/root", Automatic: true, Unmetered: true})
	u.window.Show()
	u.reviewSetup(settings{DiscoveryURL: "https://sync.example", DiscoveryToken: strings.Repeat("t", 32), FolderID: "camera", Secret: strings.Repeat("s", 32)})
	if u.folder.Text != "" {
		t.Fatal("QR imported before confirmation")
	}
	var find func(fyne.CanvasObject) *widget.Button
	seen := map[fyne.CanvasObject]bool{}
	find = func(o fyne.CanvasObject) *widget.Button {
		if o == nil || seen[o] {
			return nil
		}
		seen[o] = true
		if b, ok := o.(*widget.Button); ok && b.Text == "Yes" {
			return b
		}
		var children []fyne.CanvasObject
		switch v := o.(type) {
		case *fyne.Container:
			children = v.Objects
		case fyne.Widget:
			children = test.WidgetRenderer(v).Objects()
		}
		for _, c := range children {
			if b := find(c); b != nil {
				return b
			}
		}
		return nil
	}
	b := find(u.window.Canvas().Overlays().Top())
	if b == nil {
		t.Fatal("confirmation button missing")
	}
	test.Tap(b)
	if u.folder.Text != "camera" || u.device.Text != "my-phone" || u.cameraURI != "content://camera/tree/root" || !u.automatic.Checked || !u.unmetered.Checked {
		t.Fatal("QR replaced phone settings")
	}
	if !u.dirty {
		t.Fatal("QR was saved without an explicit save")
	}
}
func TestBridgeErrorAndUnicode(t *testing.T) {
	if decodeResponse(`{"error":"Choose the camera folder"}`, nil) == nil {
		t.Fatal("bridge error ignored")
	}
	var s snapshot
	if err := decodeResponse(`{"message":"Saved café 📸.jpg"}`, &s); err != nil || s.Message != "Saved café 📸.jpg" {
		t.Fatal("UTF-8 status did not round trip")
	}
}
