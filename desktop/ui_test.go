// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fyne.io/fyne/v2/test"
	"image/png"
	"os"
	"testing"
)

func TestDesktopUsesExistingTOMLAndWaitsForExplicitStart(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	filename := desktopConfig(t)
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	u := newUI(a, filename)
	if u.editor.Text != string(raw) {
		t.Fatal("GUI did not load the existing TOML verbatim")
	}
	if _, running := u.controller.snapshot(); running {
		t.Fatal("GUI started another engine without an explicit request")
	}
	u.window.Show()
	if filename := os.Getenv("BRUNDLESYNC_SCREENSHOT"); filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, u.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
}
