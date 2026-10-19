# Android camera backup

The Android app uses a Fyne interface in Go, with Android adapters for QR scanning,
folder permissions and WorkManager around the shared Go engine. Its first release provides one-way photo/video backup from a
user-selected camera folder. It does not mirror desktop folders onto the phone.

The phone initiates direct, authenticated uploads to a reachable desktop client.
It needs no incoming port, discovery lease or continuously running listener.
The discovery server remains a directory only; photos never pass through it.

## Set up the desktop

1. Update a receiving client to 0.5.0 or newer.
2. Create the backup destination directory.
3. Enable `receive_backups = true` for its shared folder, using the desktop GUI
   or TOML. See [Desktop setup](DESKTOP.md).
4. Start that desktop client and keep it reachable from the phone: LAN, a
   routable mapped endpoint, or a VPN. Discovery does not provide a data relay.

## Build the APK

Requirements: Go 1.26+, JDK 17+, Android SDK command-line tools, SDK platform 36,
Build Tools 35.0.0, and NDK 28.2.13676358. Android Studio's SDK Manager can install
these. The app supports Android 7.0/API 24 and newer, with ARM, ARM64 and x86-64
native libraries.

```sh
export ANDROID_HOME="$HOME/Android/Sdk"  # use your actual SDK directory
sdkmanager 'platforms;android-36' 'build-tools;35.0.0' 'ndk;28.2.13676358'
make android
```

The output is `android/app/build/outputs/apk/debug/app-debug.apk`.
Install it with Android Studio/ADB, or transfer it to your phone and open it:

```sh
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

The Gradle wrapper is pinned and checksum verified. Gradle builds the Go worker AAR
and the Fyne NativeActivity libraries automatically. Fyne is pinned in `phoneui/go.mod`;
the binding tools are pinned in `mobile/go.mod`. No separate Fyne CLI installation is needed. To build just
the native library, run `bash scripts/build-android-core.sh`. `GO` and
`ANDROID_NDK_HOME` can override their defaults. On Windows, use Git Bash or WSL
with SDK/NDK tools for that environment, or build through the Linux CI job.

The **Desktop and Android** workflow publishes a debug APK after its checks.
These are test builds, not Play Store releases. CI debug signing keys can differ
between runs; for ongoing installations, keep the same local debug keystore or
sign a release with your own stable key. Android cannot update an installed app
with a different signing key without uninstalling it; uninstalling clears its
settings, grants and local receipt index.

## Configure the phone

1. In desktop **Folders → Set up Android**, show the QR for a folder that accepts backups.
2. In Android's Fyne **Settings** tab, tap **Scan desktop QR**. Confirm the server and destination in the Fyne review dialog. Manual connection fields remain available.
3. Tap **Choose camera folder**, then select the actual camera directory, commonly `DCIM/Camera`. Android grants read access to that directory.
4. Choose automatic backup, network and charging requirements.
5. Tap **Save & start backup**. The **Status** tab shows configuration readiness and the queued/running state; **Activity** keeps recent events.

The package ID, private TOML, camera grant, device ID and receipt database are unchanged from 0.6.0. Upgrade using the same signing key; no configuration reset or discovery-server update is required. The UI migration does not change file transfer concurrency or the upload protocol.

No all-files access or broad photo-library permission is requested. The saved
folder grant is read-only and survives process restarts/reboots. If Android
revokes it or storage changes, choose the folder again.

The app owns `files/config.toml` in its private app storage. It uses the shared
TOML schema and validation, with an `[android]` section holding the camera URI
and scheduling preferences. A content URI is a platform grant, not a portable
filesystem path. Desktop and Android have their own device identities and local
paths; do not copy desktop identity files onto the phone.

## Backup behavior

- Only images and videos in the selected tree are considered; subfolders are
  included. Ordinary local camera files upload through granted file descriptors
  without a temporary copy on the phone.
- Non-seekable providers use one bounded temporary staging file. Available space
  and the configured file-size limit are checked; temporary copies are removed.
- Uploads use 1 MiB independently verified chunks. The desktop flushes each
  checkpoint, then verifies the whole file and records its history before giving
  a completed receipt. A retry resumes the acknowledged prefix, including after
  a receiver restart. Incomplete files never appear as finished shared files.
- The phone records success only after that completed receipt. Its SQLite index
  is scoped to the source selection, destination credentials and device ID.
- **Removing a phone photo never deletes the desktop backup or restores the photo
  onto the phone.** The app never deletes originals. Desktop copies can still be
  managed or deleted by you and by the desktop folder's ordinary sync rules.
- Existing backup paths with different contents are never overwritten. Hash
  suffixes prevent camera filename collisions. A completed file is immutable to
  this upload API; normal desktop editing remains possible.
- Recent files are deferred briefly to avoid backing up an active recording.
- **Recheck previously backed-up files** reads sources again, useful after
  edits with preserved metadata or after removing a desktop copy. A recheck job
  retains progress across retries instead of repeatedly starting at the beginning.
- The default maximum file size is 10 GiB, shared with the engine. Files over it
  are reported and skipped. The private TOML can adjust `max_file_size`.

## Background work

Automatic backup uses native WorkManager jobs, with a 15-minute requested period.
Android controls their actual timing and may defer them for battery, networking,
standby or device-specific reasons. Back up now uses a one-time job and respects
the saved network/charging choices. Battery-low protection applies to both.

Jobs stop before the normal background-work time limit and can continue through
resumable chunks and durable file receipts. Cancellation closes native source
streams and cancels the Go network context. A lost connection does not mark a
photo as backed up. Stop backups disables automatic scheduling and cancels active
work; save settings to enable it again.

The app does not rely on an Activity staying alive, an always-running foreground
service, or a desktop-style fsnotify loop. Notification permission is optional;
without it, the last result remains visible in the app.

The receiver permits up to 64 unfinished files per folder. Abandoned partials
older than seven days are collected at receiver startup or when a new partial is
created. Only a completed receipt is a finished backup.

## Validation and limits

Go tests cover direct pinned-TLS transfer, idempotence, collisions, interrupted
history, restart recovery, bad authentication, corrupt chunks and final hashes,
and verified-prefix resume across receiver restarts. Android unit tests cover
URI-based media enumeration and receipt-index invalidation/resume behavior.

Native emulator tests load the Go library, test cancellation, and transfer a
multi-chunk file descriptor to real desktop/discovery processes. To run them on
a host with KVM, install the API 35 Google APIs x86-64 image and emulator, build
the CLI, then run `bash scripts/android-emulator-test.sh`. The SDK also needs its
usual accepted licenses. That script uses test-only HTTP discovery on the
emulator's host loopback alias; the content transfer still uses pinned TLS.

Physical camera-provider behavior, SD cards, long videos, reboot scheduling and
OEM battery policies still need field testing on real phones. This initial
Android app is a camera-backup preview, with no general bidirectional folder UI,
relay, or cloud storage service.

## QR setup and backup status (0.6.0)

1. On desktop, add a folder with **Accept Android camera backups** enabled (or set `receive_backups = true` on an existing folder). Validate and save the TOML.
2. In **Folders → Set up Android**, select that destination and show the QR. Keep desktop sync running to receive files.
3. On Android, open **Settings → Scan desktop QR** and grant camera access. Review the server and folder, then select **Use setup**. No configuration is saved yet.
4. Choose your camera folder with Android's folder picker. Choose automatic backup, network and charging preferences, then **Save & start backup**. The app saves its TOML and queues an initial backup using those constraints. Periodic backup starts no earlier than 15 minutes afterward; Android determines the actual schedule.

**Configured** means valid saved settings and retained camera folder permission. It does not claim an always-connected desktop. **Status** shows WorkManager's active/queued state, current file, saved/unchanged/skipped counts and last completed backup check. Missing folder permission and retry conditions remain visible. Counts are per check, not lifetime totals; progress is per file rather than an invented byte percentage. Unsaved changes disable manual backup.

**Activity** stores the latest 100 events locally, including saved files and backup checks. Clearing it does not clear backup receipts or cause reuploads. **Settings** keeps manual configuration available behind a button. Scanning a QR never replaces the phone's device ID, camera folder or scheduling choices. Invalid codes leave fields unchanged.

The QR contains reusable discovery and selected-folder credentials, not a one-time invitation. Show it privately; anyone who receives it has those credentials until you rotate them. QR generation is local and excludes desktop identities, filesystem paths and other folders. HTTPS discovery is required for QR setup. Existing manual settings remain supported. Camera permission is only needed to scan; file access continues to use the folder picker. No VPS update is required.

## Fyne interface (0.7.0)

The app's screens, forms, QR confirmation and tabs are Fyne widgets in `phoneui`. Android's brief permission/picker activities remain native. WorkManager and the existing Go upload engine stay in the app process; the Fyne renderer runs in a separate `:ui` process. This avoids loading two different Go shared-library runtimes into one process and lets backup continue when the UI closes. A private same-UID ContentProvider carries settings/status commands and change notifications. Only foreground UI observers refresh the screens; there is no idle polling timer.

`make android` still builds the whole APK. `scripts/build-android-ui.sh` compiles ARM, ARM64 and x86-64 Fyne libraries with the installed NDK and copies the unmodified Activity classes from the pinned Fyne module into generated build directories. Fyne is used under its BSD license, included in CI artifacts.
