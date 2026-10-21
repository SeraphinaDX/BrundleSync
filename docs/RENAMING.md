# Upgrading to BrundleSync

BrundleSync 0.8.0 is the new name for DoucheSync. The CLI, desktop window, tray
menu, Android launcher label, QR prompts and backup notifications now use the new
name. The project remains GPL-3.0-or-later.

## Commands and services

Build with `make build` and `make gui` as before. The outputs are now
`BrundleSync` and `BrundleSyncGUI` (`.exe` on Windows). Subcommands and TOML
settings are unchanged:

```sh
./BrundleSync version
./BrundleSync client
./BrundleSyncGUI
```

Stop the old client before starting the new executable. Both programs use the
same configuration, identity and folder locks; do not run both against the same
folders. No configuration copy or history reset is needed.

Existing systemd units keep their old names until you choose to replace them.
Update their `ExecStart` paths to the new executable and retain their existing
arguments, users, configuration locations and permissions. Run `daemon-reload`
and restart the relevant unit after editing it (`systemctl --user` for a user
client, `sudo systemctl` for a system server).

If you want to keep an existing service completely unchanged, install the new
binary at its existing executable path. For example, an old discovery service
using `/usr/local/bin/DoucheSync` can continue using that path:

```sh
sudo install -m 755 BrundleSync /usr/local/bin/DoucheSync
sudo systemctl restart douchesync-server.service
```

New service examples are `examples/brundlesync-client.service` and
`examples/brundlesync-server.service`. Do not enable a second service alongside
an existing one. The server example retains the existing `douchesync` system
account and `/etc/douchesync/server.toml` location for compatibility.

## Saved data and peer compatibility

The following identifiers deliberately retain their old spelling:

- Default TOML directory (`douchesync` under the OS configuration directory).
- Default identity directory and each shared folder's `.douchesync` history.
- Desktop application ID and launcher file ID.
- Android application ID `ca.cerberusgames.douchesync`, private storage, folder
  grants, WorkManager state and internal native-library/JNI identifiers.
- Authenticated peer protocol headers, discovery group/signature formats,
  camera-setup QR envelope and router mapping ownership labels.

Keep these locations and identifiers intact. This lets renamed clients use
existing settings and histories, recognize existing QR codes and communicate
with older compatible clients and discovery servers. Updating the discovery
server to change its displayed version/name is optional for this rename.

## Android and Obtainium

Release assets are now named `BrundleSync-v<VERSION>.apk`. The release workflow
uses the same GitHub signing secrets and certificate fingerprint. **Do not
regenerate the signing key or change its alias because of the rename.** Existing
`~/.config/douchesync-signing/release.p12` files remain valid.

Install the new release as an update over an existing permanent-key-signed APK;
the package ID and increasing version code allow an in-place upgrade. A change
in the app's display name alone does not require uninstalling. An installation
still signed with an older, different debug key remains subject to the one-time
[signing migration](ANDROID-SIGNING.md#existing-installations).

Keep Obtainium tracking this repository. If you configured an asset-name filter
containing `DoucheSync`, update it to match `BrundleSync` (or use an APK-extension
filter). A checksum file ending in `.apk.sha256` is not an APK.

The repository URL in documentation remains the current GitHub address. Renaming
the repository itself is a separate GitHub Settings operation; it is not part of
the source rename. After a repository rename, check Obtainium's source URL and
update Git remotes as appropriate.
