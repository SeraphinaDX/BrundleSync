// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 DoucheSync contributors
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"brundlesync/engine"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed assets/brundlesync.svg
var applicationIcon []byte

type desktopUI struct {
	app                                                         fyne.App
	window                                                      fyne.Window
	controller                                                  controller
	configPath                                                  string
	state, device, problem, folders, peers, transfers, activity *widget.Label
	editor                                                      *widget.Entry
	start, stop, syncNow, save, add                             *widget.Button
	quitOnce                                                    sync.Once
	configured                                                  engine.Config
}

func main() {
	config := flag.String("config", engine.DefaultConfigPath(), "existing BrundleSync TOML file")
	autoStart := flag.Bool("start", false, "start syncing immediately")
	flag.Parse()
	a := app.NewWithID("ca.cerberusgames.douchesync")
	a.SetIcon(fyne.NewStaticResource("brundlesync.svg", applicationIcon))
	u := newUI(a, *config)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s, running := u.controller.snapshot()
				fyne.Do(func() { u.render(s, running) })
			}
		}
	}()
	u.window.SetOnClosed(cancel)
	go func() { <-ctx.Done(); fyne.Do(u.quit) }()
	if *autoStart {
		if err := u.controller.start(*config); err != nil {
			dialog.ShowError(err, u.window)
		}
	}
	u.window.ShowAndRun()
	<-u.controller.stop()
}

func newUI(a fyne.App, configPath string) *desktopUI {
	a.Settings().SetTheme(applicationTheme{theme.DefaultTheme()})
	u := &desktopUI{app: a, window: a.NewWindow("BrundleSync " + engine.Version()), configPath: configPath}
	u.state = widget.NewLabel("Stopped")
	u.device = widget.NewLabel("")
	u.problem = widget.NewLabel("")
	u.problem.Wrapping = fyne.TextWrapWord
	u.folders = widget.NewLabel("")
	u.folders.Wrapping = fyne.TextWrapWord
	u.peers = widget.NewLabel("")
	u.peers.Wrapping = fyne.TextWrapWord
	u.transfers = widget.NewLabel("No active transfers")
	u.transfers.Wrapping = fyne.TextWrapWord
	u.activity = widget.NewLabel("")
	u.activity.Wrapping = fyne.TextWrapWord
	u.start = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		if err := u.controller.start(u.configPath); err != nil {
			dialog.ShowError(err, u.window)
		}
		u.refresh()
	})
	u.stop = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() { u.controller.stop(); u.refresh() })
	u.syncNow = widget.NewButtonWithIcon("Sync now", theme.ViewRefreshIcon(), u.controller.syncNow)
	u.editor = widget.NewMultiLineEntry()
	u.editor.Wrapping = fyne.TextWrapOff
	u.loadConfig()
	u.save = widget.NewButtonWithIcon("Validate and save", theme.DocumentSaveIcon(), func() {
		if _, running := u.controller.snapshot(); running {
			dialog.ShowInformation("Stop sync first", "Stop syncing before changing configuration.", u.window)
			return
		}
		if err := engine.SaveConfigText(u.configPath, u.editor.Text); err != nil {
			dialog.ShowError(err, u.window)
			return
		}
		u.configured, _ = engine.DecodeConfig(u.editor.Text)
		dialog.ShowInformation("Configuration saved", "The TOML is valid. Start sync to apply it.", u.window)
	})
	u.add = widget.NewButtonWithIcon("Add folder", theme.FolderNewIcon(), u.addFolder)
	open := widget.NewButtonWithIcon("Open config directory", theme.FolderOpenIcon(), func() {
		p, _ := filepath.Abs(filepath.Dir(u.configPath))
		_ = a.OpenURL(&url.URL{Scheme: "file", Path: filepath.ToSlash(p)})
	})
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Overview", theme.HomeIcon(), container.NewVScroll(container.NewVBox(widget.NewLabelWithStyle("Direct peer-to-peer sync", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), u.device, u.state, u.problem, widget.NewSeparator(), widget.NewLabel("Transfers"), u.transfers))),
		container.NewTabItemWithIcon("Folders", theme.FolderIcon(), container.NewBorder(nil, container.NewHBox(u.add, widget.NewButtonWithIcon("Set up Android", theme.ComputerIcon(), u.showAndroidSetup)), nil, nil, container.NewVScroll(u.folders))),
		container.NewTabItemWithIcon("Peers", theme.ComputerIcon(), container.NewVScroll(u.peers)),
		container.NewTabItemWithIcon("Activity", theme.HistoryIcon(), container.NewVScroll(u.activity)),
		container.NewTabItemWithIcon("Configuration", theme.SettingsIcon(), container.NewBorder(container.NewVBox(widget.NewLabel(u.configPath), widget.NewLabel("Uses the CLI's TOML. Stop sync before saving changes.")), container.NewHBox(u.save, open), nil, nil, u.editor)),
	)
	u.window.SetContent(container.NewBorder(container.NewHBox(u.start, u.stop, u.syncNow), nil, nil, nil, tabs))
	u.window.Resize(fyne.NewSize(920, 620))
	if tray, ok := a.(desktop.App); ok {
		tray.SetSystemTrayMenu(fyne.NewMenu("BrundleSync", fyne.NewMenuItem("Show", u.window.Show), fyne.NewMenuItem("Sync now", u.controller.syncNow), fyne.NewMenuItem("Quit", u.quit)))
		tray.SetSystemTrayWindow(u.window)
	}
	u.window.SetCloseIntercept(u.quit)
	u.window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("File", fyne.NewMenuItem("Quit", u.quit))))
	u.refresh()
	return u
}
func (u *desktopUI) quit() {
	u.quitOnce.Do(func() {
		u.state.SetText("Stopping…")
		done := u.controller.stop()
		go func() { <-done; fyne.Do(u.app.Quit) }()
	})
}
func (u *desktopUI) refresh() { s, running := u.controller.snapshot(); u.render(s, running) }
func (u *desktopUI) render(s engine.Status, running bool) {
	u.state.SetText("Status: " + s.Phase)
	device := s.Device
	if device == "" {
		device = u.configured.Client.DeviceID
	}
	u.device.SetText("Device: " + device + "   " + s.Address)
	if s.LastError == "" {
		u.problem.Hide()
	} else {
		u.problem.SetText(s.LastError)
		u.problem.Show()
	}
	if running {
		u.start.Disable()
		u.save.Disable()
		u.add.Disable()
		u.stop.Enable()
		u.syncNow.Enable()
	} else {
		u.start.Enable()
		u.save.Enable()
		u.add.Enable()
		u.stop.Disable()
		u.syncNow.Disable()
	}
	var folders, peers, transfers, events []string
	for _, f := range s.Folders {
		state := "ready"
		if !f.Healthy {
			state = "needs attention"
		}
		if f.Busy {
			state = "scanning / updating"
		}
		backup := ""
		if f.ReceiveBackups {
			backup = " · accepts Android camera backups"
		}
		folders = append(folders, fmt.Sprintf("%s\n%s\n%d files · %d deletions · %s%s\n", f.ID, f.Path, f.Files, f.Deleted, state, backup))
	}
	for _, p := range s.Peers {
		peers = append(peers, fmt.Sprintf("%s · %s\n%s · %s\n", p.Device, p.Folder, p.Address, p.State))
	}
	for _, t := range s.Transfers {
		transfers = append(transfers, fmt.Sprintf("%s · %s/%s\n%.1f / %.1f MiB", t.Direction, t.Folder, t.Path, float64(t.Bytes)/(1<<20), float64(t.Total)/(1<<20)))
	}
	for i := len(s.Events) - 1; i >= 0; i-- {
		e := s.Events[i]
		events = append(events, e.Time.Format("15:04:05")+"  "+e.Message)
	}
	if len(folders) == 0 {
		for _, f := range u.configured.Folders {
			folders = append(folders, f.ID+"\n"+f.Path+"\n")
		}
	}
	if len(peers) == 0 {
		peers = append(peers, "No peers discovered yet")
	}
	if len(transfers) == 0 {
		transfers = append(transfers, "No active transfers")
	}
	u.folders.SetText(strings.Join(folders, "\n"))
	u.peers.SetText(strings.Join(peers, "\n"))
	u.transfers.SetText(strings.Join(transfers, "\n"))
	u.activity.SetText(strings.Join(events, "\n"))
}
func (u *desktopUI) loadConfig() {
	b, err := os.ReadFile(u.configPath)
	if err == nil {
		u.editor.SetText(string(b))
		u.configured, _ = engine.DecodeConfig(string(b))
		return
	}
	u.editor.SetText("[client]\ndevice_id = \"desktop\"\nlisten = \":7444\"\ndiscovery_url = \"https://sync.example.com\"\ndiscovery_token = \"REPLACE_WITH_YOUR_DISCOVERY_TOKEN\"\n\n# Use Add folder to configure an existing local folder.\n")
}
func (u *desktopUI) addFolder() {
	id, pathEntry, secret := widget.NewEntry(), widget.NewEntry(), widget.NewPasswordEntry()
	secret.SetText(engine.GenerateSecret())
	browse := widget.NewButton("Browse…", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, u.window)
			} else if uri != nil {
				pathEntry.SetText(uri.Path())
			}
		}, u.window)
	})
	backups := widget.NewCheck("Accept Android camera backups", nil)
	deletes := widget.NewCheck("Propagate deletions", nil)
	form := dialog.NewForm("Add shared folder", "Add to TOML", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Folder ID", id), widget.NewFormItem("Local folder", container.NewBorder(nil, nil, nil, browse, pathEntry)), widget.NewFormItem("Shared secret", secret), widget.NewFormItem("", backups), widget.NewFormItem("", deletes),
	}, func(ok bool) {
		if !ok {
			return
		}
		// Encode just the appended section; preserve every existing comment.
		text, err := engine.AppendFolderConfig(u.editor.Text, engine.FolderConfig{ID: id.Text, Path: pathEntry.Text, Secret: secret.Text, SyncDeletes: deletes.Checked, ReceiveBackups: backups.Checked})
		if err != nil {
			dialog.ShowError(err, u.window)
			return
		}
		u.editor.SetText(text)
		dialog.ShowInformation("Folder added to editor", "Use Validate and save in Configuration, then Start. Use the same folder ID and secret on its peers.", u.window)
	}, u.window)
	form.Resize(fyne.NewSize(650, 340))
	form.Show()
}
