// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAndroidSetupRoundTrip(t *testing.T) {
	cfg := Config{Client: ClientConfig{DeviceID: "private-desktop", DiscoveryURL: "https://sync.example.com", DiscoveryToken: strings.Repeat("t", 32)}, Folders: []FolderConfig{{ID: "camera", Path: "/private/path", Secret: strings.Repeat("s", 32), ReceiveBackups: true}, {ID: "other", Secret: strings.Repeat("x", 32)}}}
	code, err := EncodeAndroidSetup(cfg, "camera")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeAndroidSetup(code)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != "camera" || got.Secret != cfg.Folders[0].Secret {
		t.Fatalf("wrong destination: %s", got.FolderID)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, androidSetupPrefix))
	for _, private := range []string{"private-desktop", "/private/path", strings.Repeat("x", 32)} {
		if strings.Contains(string(raw), private) {
			t.Fatal("leaked unrelated settings")
		}
	}
	cfg.Folders[0].ReceiveBackups = false
	if _, err := EncodeAndroidSetup(cfg, "camera"); err == nil {
		t.Fatal("backup opt-in required")
	}
}

func TestAndroidSetupRejectsInvalid(t *testing.T) {
	for _, raw := range []string{
		`{"discoveryURL":"http://sync.example.com","discoveryToken":"tttttttttttttttttttttttttttttttt","folderID":"camera","secret":"ssssssssssssssssssssssssssssssss"}`,
		`{"discoveryURL":"https://sync.example.com","discoveryToken":"short","folderID":"camera","secret":"ssssssssssssssssssssssssssssssss"}`,
		`{"unknown":true}`, `{}`, `{} {}`, `not json`,
	} {
		if _, err := DecodeAndroidSetup(androidSetupPrefix + base64.RawURLEncoding.EncodeToString([]byte(raw))); err == nil {
			t.Fatal("accepted invalid setup")
		}
	}
	for _, code := range []string{"", "DOUCHESYNC-CAMERA-2:abc", androidSetupPrefix + "!", strings.Repeat("a", maxAndroidSetup+1)} {
		if _, err := DecodeAndroidSetup(code); err == nil {
			t.Fatal("accepted invalid envelope")
		}
	}
}
