# Desktop GUI

The desktop app uses Fyne and the same Go engine, TOML file, folder history and
saved TLS identity as the CLI. Linux and Windows builds are checked in CI.

## Build and run

Use Go 1.26 or newer. GUI dependencies download on the first build; the headless
CLI/server keeps its independent vendored build and does not require graphics.

On Debian/Ubuntu, install the graphics development packages:

```sh
sudo apt-get install build-essential pkg-config libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev libegl1-mesa-dev
```

On Arch/CachyOS:

```sh
sudo pacman -S --needed base-devel pkgconf libglvnd libxcursor libxrandr libxinerama libxi libxxf86vm wayland libxkbcommon
```

From the checkout root:

```sh
make gui
./DoucheSyncGUI
```

An alternative Go/cgo wrapper can be selected with `make gui GUI_GO=zgo`.
On Linux, `make install-gui` installs the executable, desktop launcher and SVG
icon under `~/.local`; that directory's `bin` must be on your PATH.

On Windows, install a MinGW-w64 C compiler, add its `bin` directory to PATH, then
use PowerShell:

```powershell
cd desktop
$env:CGO_ENABLED = "1"
go build -trimpath -ldflags="-H=windowsgui" -o ..\DoucheSyncGUI.exe .
cd ..
.\DoucheSyncGUI.exe
```

macOS requires Xcode command-line tools. The UI uses Fyne's platform support;
macOS native runtime testing is not part of this release's CI.

## Existing configuration

The default path is the CLI's normal configuration path:

| Platform | Path |
| --- | --- |
| Linux | `~/.config/douchesync/config.toml` (or `$XDG_CONFIG_HOME/douchesync/config.toml`) |
| Windows | `%AppData%\douchesync\config.toml` |
| macOS | `~/Library/Application Support/douchesync/config.toml` |

To select another file or start syncing immediately:

```sh
./DoucheSyncGUI -config=/absolute/path/client.toml -start
```

Stop the CLI client/service before starting sync in the GUI. One engine owns
the device identity, folder locks and listening port. The GUI does not start a
second daemon or silently change IDs to avoid a lock. Keep existing IDs, secrets
and `.douchesync` histories unchanged.

## Controls

- **Start / Stop:** start an engine or cancel it and release its locks. Starting
  again performs startup verification, just like restarting the CLI.
- **Sync now:** request a metadata scan and peer check without discarding the
  verified hash cache.
- **Overview:** phase, device address, errors and active transfer byte progress.
- **Folders:** paths, file/deletion counts and Android backup reception.
- **Peers:** discovered peers and their last reachability check; this does not
  mean an idle TCP connection is permanently open.
- **Activity:** a bounded history of verified transfers and errors.
- **Configuration:** edit the actual TOML. Saving validates first, preserves
  comments and formatting, and replaces the file atomically. Stop sync first.
- **Add folder:** choose an existing directory and append its configuration to
  the editor. Save it in Configuration, then Start. IDs/secrets must match peers.

Closing the main window or choosing Quit stops sync before exiting. A system
tray menu is available on supporting desktops. The GUI does not install an
autostart service or change your existing service configuration.

Folder counts are cached; the UI does not walk folders or hash files to refresh
its display. A busy folder is marked while verification/mutations hold its lock.

## Accept Android camera backups

Use a dedicated shared folder if you want camera backups separate from ordinary
documents. Enable the checkbox when adding it, or add this to its existing TOML
section:

```toml
[[folders]]
id = "camera"
path = "~/Pictures/PhoneBackups"
secret = "THE_SAME_FOLDER_SECRET_USED_ON_ANDROID"
receive_backups = true
sync_deletes = false
```

The directory must already exist. Match its ID/secret in Android and use the same
discovery URL/token. The receiving desktop must run 0.5.0 or newer. Discovery
servers from 0.2.0 onward remain compatible and store no photo contents.

Backups appear beneath `Camera/<phone-device-id>/`. Filenames have a readable
stem and full content-hash suffix, preserving different photos with identical
camera filenames. Completed files can synchronize to your other desktop peers
normally. Partial uploads stay private and never enter folder manifests.
