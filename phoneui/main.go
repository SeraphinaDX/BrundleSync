// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"image/color"
	"sync"
)

type settings struct {
	DeviceID       string `json:"deviceID"`
	DiscoveryURL   string `json:"discoveryURL"`
	DiscoveryToken string `json:"discoveryToken"`
	FolderID       string `json:"folderID"`
	Secret         string `json:"secret"`
	CameraURI      string `json:"cameraURI"`
	Automatic      bool   `json:"automatic"`
	Unmetered      bool   `json:"unmetered"`
	Charging       bool   `json:"charging"`
}
type snapshot struct {
	Configured bool      `json:"configured"`
	Settings   *settings `json:"settings"`
	Ready      bool      `json:"ready"`
	Running    bool      `json:"running"`
	State      string    `json:"state"`
	Message    string    `json:"message"`
	Saved      int       `json:"saved"`
	Unchanged  int       `json:"unchanged"`
	Skipped    int       `json:"skipped"`
	Completed  string    `json:"completed"`
	History    string    `json:"history"`
}
type results struct {
	Setup     *settings `json:"setup"`
	CameraURI string    `json:"cameraURI"`
	Notice    string    `json:"notice"`
}

// Android owns TOML, permissions and background work. This narrow bridge keeps
// the screens testable without reimplementing any backup logic in the UI.
type backend interface {
	Request(string, any, any) error
	Launch(string) error
}
type phoneUI struct {
	window                                                                                  fyne.Window
	backend                                                                                 backend
	tabs                                                                                    *container.AppTabs
	headline, state, message, summary, counts, completed, history, source, checklist, edits *widget.Label
	progress                                                                                *widget.ProgressBarInfinite
	device, server, token, folder, secret                                                   *widget.Entry
	automatic, unmetered, charging                                                          *widget.Check
	save, backup, recheck                                                                   *widget.Button
	cameraURI                                                                               string
	dirty, populating, saving, loaded                                                       bool
	revision                                                                                int
	saved                                                                                   snapshot
	requests                                                                                sync.Mutex
}

func main() {
	a := app.NewWithID("ca.cerberusgames.douchesync")
	a.Settings().SetTheme(phoneTheme{theme.DefaultTheme()})
	u := newPhoneUI(a, platformBackend{})
	go func() {
		for range platformChanges {
			u.reload(true)
		}
	}()
	a.Lifecycle().SetOnEnteredForeground(signalChange)
	signalChange()
	u.window.ShowAndRun()
}
func wrapped(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// Fyne overlays its vertical scroll bar instead of reserving content width.
// Keep controls and text inset from that track, including the expanded thumb.
func phoneContent(content fyne.CanvasObject) *fyne.Container {
	padding := theme.Padding() * 2
	return container.New(layout.NewCustomPaddedLayout(padding, padding, padding, padding+theme.ScrollBarSize()), content)
}

func phoneScroll(content fyne.CanvasObject) *container.Scroll {
	return container.NewVScroll(phoneContent(content))
}

func newPhoneUI(a fyne.App, b backend) *phoneUI {
	u := &phoneUI{window: a.NewWindow("DoucheSync"), backend: b}
	u.headline = widget.NewLabelWithStyle("Setup needed", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.state = wrapped("Set up your camera backup")
	u.message = wrapped("")
	u.summary = wrapped("")
	u.counts = wrapped("")
	u.completed = wrapped("")
	u.history = wrapped("No backup activity yet.")
	u.source = wrapped("")
	u.checklist = wrapped("")
	u.edits = wrapped("")
	u.progress = widget.NewProgressBarInfinite()
	u.progress.Stop()
	u.progress.Hide()
	u.device = widget.NewEntry()
	u.server = widget.NewEntry()
	u.token = widget.NewPasswordEntry()
	u.folder = widget.NewEntry()
	u.secret = widget.NewPasswordEntry()
	u.automatic = widget.NewCheck("Back up automatically", func(bool) { u.edited() })
	u.unmetered = widget.NewCheck("Use unmetered networks only", func(bool) { u.edited() })
	u.charging = widget.NewCheck("Only while charging", func(bool) { u.edited() })
	u.populating = true
	u.unmetered.SetChecked(true)
	u.populating = false
	for _, e := range []*widget.Entry{u.device, u.server, u.token, u.folder, u.secret} {
		e.OnChanged = func(string) { u.edited() }
	}
	u.save = widget.NewButtonWithIcon("Save & start backup", theme.ConfirmIcon(), u.saveSettings)
	u.save.Importance = widget.HighImportance
	u.backup = widget.NewButtonWithIcon("Back up now", theme.MediaPlayIcon(), func() { u.action("backup") })
	u.backup.Importance = widget.HighImportance
	u.recheck = widget.NewButtonWithIcon("Recheck all photos", theme.ViewRefreshIcon(), func() {
		dialog.ShowConfirm("Recheck camera files?", "This verifies every camera file again and can use more data.", func(ok bool) {
			if ok {
				u.action("recheck")
			}
		}, u.window)
	})
	status := phoneScroll(container.NewVBox(
		widget.NewCard("Camera backup", "Your photos, directly to your desktop", container.NewVBox(u.headline, u.state, u.progress, u.message)),
		widget.NewCard("Latest check", "", container.NewVBox(u.counts, u.completed)), u.edits, u.backup,
		container.NewAdaptiveGrid(2, widget.NewButtonWithIcon("Stop backups", theme.MediaStopIcon(), func() { u.action("stop") }), widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() { u.tabs.SelectIndex(2) })), u.recheck, widget.NewCard("Destination", "", u.summary)))
	activity := container.NewBorder(phoneContent(container.NewVBox(wrapped("Recent activity · latest 100 events"), widget.NewButtonWithIcon("Clear activity", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Clear activity?", "Backup receipts will be kept; your files will not be uploaded again.", func(ok bool) {
			if ok {
				u.action("clear")
			}
		}, u.window)
	}))), nil, nil, nil, phoneScroll(u.history))
	scan := widget.NewButtonWithIcon("Scan desktop QR", theme.MediaPhotoIcon(), func() { u.launch("scan") })
	scan.Importance = widget.HighImportance
	form := widget.NewForm(widget.NewFormItem("Phone ID", u.device), widget.NewFormItem("Discovery server", u.server), widget.NewFormItem("Discovery token", u.token), widget.NewFormItem("Folder ID", u.folder), widget.NewFormItem("Folder secret", u.secret))
	setup := phoneScroll(container.NewVBox(widget.NewCard("Set up camera backup", "Three steps, then you're ready", u.checklist), scan, widget.NewAccordion(widget.NewAccordionItem("Manual connection settings", form)), widget.NewButtonWithIcon("Choose camera folder", theme.FolderOpenIcon(), func() { u.launch("folder") }), u.source, widget.NewCard("When to back up", "Android schedules background work.", container.NewVBox(u.automatic, u.unmetered, u.charging)), u.save, wrapped("Only scan your own desktop's QR. It contains reusable access credentials. Original camera files stay on your phone.")))
	u.tabs = container.NewAppTabs(container.NewTabItemWithIcon("Status", theme.HomeIcon(), status), container.NewTabItemWithIcon("Activity", theme.HistoryIcon(), activity), container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), setup))
	u.tabs.SetTabLocation(container.TabLocationBottom)
	u.tabs.SelectIndex(2)
	u.window.SetContent(container.NewBorder(widget.NewLabelWithStyle("DoucheSync", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}), nil, nil, nil, u.tabs))
	u.window.Resize(fyne.NewSize(390, 760))
	u.render()
	return u
}
func (u *phoneUI) edited() {
	if u.populating {
		return
	}
	u.dirty = true
	u.revision++
	u.render()
}
func (u *phoneUI) form() settings {
	return settings{u.device.Text, u.server.Text, u.token.Text, u.folder.Text, u.secret.Text, u.cameraURI, u.automatic.Checked, u.unmetered.Checked, u.charging.Checked}
}
func (u *phoneUI) populate(s settings) {
	u.populating = true
	defer func() { u.populating = false }()
	u.device.SetText(s.DeviceID)
	u.server.SetText(s.DiscoveryURL)
	u.token.SetText(s.DiscoveryToken)
	u.folder.SetText(s.FolderID)
	u.secret.SetText(s.Secret)
	u.cameraURI = s.CameraURI
	u.automatic.SetChecked(s.Automatic)
	u.unmetered.SetChecked(s.Unmetered)
	u.charging.SetChecked(s.Charging)
}
func (u *phoneUI) reload(takeResults bool) {
	u.requests.Lock()
	defer u.requests.Unlock()
	var s snapshot
	if err := u.backend.Request("snapshot", nil, &s); err != nil {
		fyne.Do(func() { dialog.ShowError(err, u.window) })
		return
	}
	var r results
	if takeResults {
		if err := u.backend.Request("results", nil, &r); err != nil {
			fyne.Do(func() { dialog.ShowError(err, u.window) })
		}
	}
	fyne.Do(func() {
		first := !u.loaded
		u.loaded = true
		u.saved = s
		if !u.dirty && !u.saving && s.Settings != nil {
			u.populate(*s.Settings)
		}
		if first && s.Configured {
			u.tabs.SelectIndex(0)
		}
		if r.CameraURI != "" {
			u.cameraURI = r.CameraURI
			u.edited()
			u.tabs.SelectIndex(2)
		}
		if r.Setup != nil {
			u.reviewSetup(*r.Setup)
		}
		if r.Notice != "" {
			dialog.ShowInformation("Camera backup", r.Notice, u.window)
		}
		u.render()
	})
}
func (u *phoneUI) reviewSetup(s settings) {
	dialog.ShowConfirm("Use this backup destination?", s.DiscoveryURL+"\nFolder: "+s.FolderID+"\n\nChoose a camera folder and save to finish.", func(ok bool) {
		if !ok {
			return
		}
		u.server.SetText(s.DiscoveryURL)
		u.token.SetText(s.DiscoveryToken)
		u.folder.SetText(s.FolderID)
		u.secret.SetText(s.Secret)
		u.edited()
		u.tabs.SelectIndex(2)
	}, u.window)
}
func (u *phoneUI) launch(action string) {
	go func() {
		if err := u.backend.Launch(action); err != nil {
			fyne.Do(func() { dialog.ShowError(err, u.window) })
		}
	}()
}
func (u *phoneUI) action(action string) {
	go func() {
		u.requests.Lock()
		err := u.backend.Request(action, nil, nil)
		u.requests.Unlock()
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, u.window)
			} else if action == "stop" {
				u.populating = true
				u.automatic.SetChecked(false)
				u.populating = false
			}
		})
		u.reload(false)
	}()
}
func (u *phoneUI) saveSettings() {
	if u.saving {
		return
	}
	s, rev := u.form(), u.revision
	u.saving = true
	u.render()
	go func() {
		u.requests.Lock()
		err := u.backend.Request("save", s, nil)
		u.requests.Unlock()
		fyne.Do(func() {
			u.saving = false
			if err != nil {
				dialog.ShowError(err, u.window)
			} else {
				if rev == u.revision {
					u.dirty = false
				}
				u.tabs.SelectIndex(0)
				u.launch("notifications")
			}
			u.render()
		})
		u.reload(false)
	}()
}
func (u *phoneUI) render() {
	s := u.saved
	headline := "Setup needed"
	if s.Configured {
		if s.Ready {
			headline = "✓ Configured"
		} else {
			headline = "Camera folder permission needed"
		}
	}
	u.headline.SetText(headline)
	u.state.SetText(s.State)
	u.message.SetText(s.Message)
	u.counts.SetText(fmt.Sprintf("%d saved · %d already backed up · %d skipped", s.Saved, s.Unchanged, s.Skipped))
	if s.Completed != "" {
		u.completed.SetText("Last completed check: " + s.Completed)
	} else {
		u.completed.SetText("No completed backup check yet")
	}
	if s.Configured && s.Settings != nil {
		cfg := s.Settings
		network := "network"
		if cfg.Unmetered {
			network = "unmetered network"
		}
		charging := ""
		if cfg.Charging {
			charging = ", charging"
		}
		u.summary.SetText(cfg.FolderID + "\n" + cfg.DiscoveryURL + "\nRequires " + network + charging + ", and battery not low.")
	} else {
		u.summary.SetText("Open Settings to connect your desktop.")
	}
	u.history.SetText(s.History)
	if s.Running {
		u.progress.Show()
		u.progress.Start()
	} else {
		u.progress.Stop()
		u.progress.Hide()
	}
	if s.Ready && !u.dirty && !s.Running && !u.saving {
		u.backup.Enable()
		u.recheck.Enable()
	} else {
		u.backup.Disable()
		u.recheck.Disable()
	}
	if u.saving {
		u.save.SetText("Saving…")
		u.save.Disable()
	} else {
		u.save.SetText("Save & start backup")
		u.save.Enable()
	}
	if u.dirty {
		u.edits.SetText("Unsaved settings · save before backing up")
	} else {
		u.edits.SetText("")
	}
	if u.cameraURI == "" {
		u.source.SetText("No camera folder selected")
	} else {
		u.source.SetText(u.cameraURI)
	}
	connection := "1. Scan your desktop QR"
	if u.server.Text != "" && u.folder.Text != "" && u.secret.Text != "" && u.token.Text != "" {
		connection = "✓ Connection details entered"
	}
	source := "2. Choose your camera folder"
	if u.cameraURI != "" {
		source = "✓ Camera folder selected"
	}
	saved := "3. Save and start backup"
	if s.Ready && !u.dirty {
		saved = "✓ Settings saved"
	}
	u.checklist.SetText(connection + "\n" + source + "\n" + saved)
}

var platformChanges = make(chan struct{}, 1)

func signalChange() {
	select {
	case platformChanges <- struct{}{}:
	default:
	}
}

type phoneTheme struct{ fyne.Theme }

func (t phoneTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNamePrimary {
		if v == theme.VariantDark {
			return color.NRGBA{181, 151, 238, 255}
		}
		return color.NRGBA{116, 84, 184, 255}
	}
	return t.Theme.Color(n, v)
}
func decodeResponse(raw string, out any) error {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		return err
	}
	if e.Error != "" {
		return fmt.Errorf("%s", e.Error)
	}
	if out != nil {
		return json.Unmarshal([]byte(raw), out)
	}
	return nil
}
