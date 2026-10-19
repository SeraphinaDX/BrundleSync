// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"douchesync/engine"
	"errors"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	qrcode "github.com/skip2/go-qrcode"
	"os"
)

func setupQRImage(code string) (*canvas.Image, error) {
	qr, err := qrcode.New(code, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	img := canvas.NewImageFromImage(qr.Image(-4))
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScalePixels
	img.SetMinSize(fyne.NewSize(360, 360))
	return img, nil
}

func (u *desktopUI) showAndroidSetup() {
	raw, err := os.ReadFile(u.configPath)
	if err != nil {
		dialog.ShowError(err, u.window)
		return
	}
	if string(raw) != u.editor.Text {
		dialog.ShowError(errors.New("validate and save your configuration before showing setup"), u.window)
		return
	}
	cfg, err := engine.LoadConfig(u.configPath)
	if err != nil {
		dialog.ShowError(err, u.window)
		return
	}
	var ids []string
	for _, f := range cfg.Folders {
		if f.ReceiveBackups {
			ids = append(ids, f.ID)
		}
	}
	if len(ids) == 0 {
		dialog.ShowInformation("Choose a backup destination", "Add a folder with Accept Android camera backups, or set receive_backups = true on an existing folder, then validate and save.", u.window)
		return
	}
	selectFolder := widget.NewSelect(ids, nil)
	selectFolder.SetSelected(ids[0])
	dialog.NewForm("Set up Android", "Show QR", "Cancel", []*widget.FormItem{widget.NewFormItem("Destination", selectFolder)}, func(ok bool) {
		if !ok {
			return
		}
		code, err := engine.EncodeAndroidSetup(cfg, selectFolder.Selected)
		if err != nil {
			dialog.ShowError(err, u.window)
			return
		}
		img, err := setupQRImage(code)
		if err != nil {
			dialog.ShowError(err, u.window)
			return
		}
		help := widget.NewLabel("On Android: Settings → Scan desktop QR.\nReview the destination, choose your camera folder,\nthen Save & start backup. Keep this desktop syncing.\n\nThis QR contains reusable access credentials.\nOnly scan it on your own phone; do not share it.")
		help.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustom("Android setup · "+selectFolder.Selected, "Close", container.NewVBox(img, help), u.window)
		d.Resize(fyne.NewSize(520, 570))
		d.Show()
	}, u.window).Show()
}
