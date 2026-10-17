# Android camera backup

The Android app is a native Kotlin interface and WorkManager adapter around the
shared Go engine. Its first release provides one-way photo/video backup from a
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

The Gradle wrapper is pinned and checksum verified. Gradle builds the Go AAR
automatically using the tool versions pinned in `mobile/go.mod`. To build just
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

1. Enter the discovery URL/token and receiving folder ID/secret.
2. Keep the generated phone device ID stable, or choose a valid ID once.
3. Tap **Choose camera folder** and select the actual camera directory, commonly
   `DCIM/Camera`. Android's system picker grants read access to that directory.
4. Choose automatic backup, unmetered networking and charging requirements.
5. Save settings, then tap **Back up now** to test the connection.

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
