// SPDX-License-Identifier: GPL-3.0-or-later
package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeSettingsShareTOMLAndScopeDestinations(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.toml")
	device := NewDeviceID()
	secret := strings.Repeat("s", 32)
	token := strings.Repeat("t", 32)
	if err := Configure(filename, device, "https://sync.example.com", token, "camera", secret, "content://photos/tree/camera", true, true, false); err != nil {
		t.Fatal(err)
	}
	text, err := Settings(filename)
	if err != nil {
		t.Fatal(err)
	}
	var settings settings
	if err = json.Unmarshal([]byte(text), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.DeviceID != device || settings.CameraURI != "content://photos/tree/camera" || !settings.Automatic {
		t.Fatal("native settings did not round trip through TOML")
	}
	a, err := NewSession(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Cancel()
	if settings.Scope != a.Scope() {
		t.Fatal("UI status scope differs from upload receipts")
	}
	if err = SetAutomatic(filename, false); err != nil {
		t.Fatal(err)
	}
	b, err := NewSession(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cancel()
	if a.Scope() != b.Scope() {
		t.Fatal("scheduling options invalidated backup receipts")
	}
	if err = Configure(filename, device, "https://sync.example.com", token, "camera", secret, "content://photos/tree/other", false, true, false); err != nil {
		t.Fatal(err)
	}
	c, err := NewSession(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Cancel()
	if a.Scope() == c.Scope() {
		t.Fatal("a different camera source reused old receipts")
	}
	a.Cancel()
	photo := filepath.Join(t.TempDir(), "photo")
	os.WriteFile(photo, []byte("photo"), 0600)
	if _, err = a.Upload(photo, "photo.jpg", 0); err == nil {
		t.Fatal("cancelled native session acknowledged a photo")
	}
}
