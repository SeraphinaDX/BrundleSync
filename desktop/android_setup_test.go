// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"douchesync/engine"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"strings"
	"testing"
)

func TestSetupQRDecodes(t *testing.T) {
	cfg := engine.Config{Client: engine.ClientConfig{DiscoveryURL: "https://sync.example.com", DiscoveryToken: strings.Repeat("t", 32)}, Folders: []engine.FolderConfig{{ID: "camera", Secret: strings.Repeat("s", 32), ReceiveBackups: true}}}
	code, err := engine.EncodeAndroidSetup(cfg, "camera")
	if err != nil {
		t.Fatal(err)
	}
	img, err := setupQRImage(code)
	if err != nil {
		t.Fatal(err)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img.Image)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := qrcode.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := engine.DecodeAndroidSetup(decoded.GetText())
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != "camera" || got.Secret != cfg.Folders[0].Secret {
		t.Fatal("wrong QR destination")
	}
}
