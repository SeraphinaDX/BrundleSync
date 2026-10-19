# Maintainer map

One Go engine owns the protocol, validation, pinned TLS, vector clocks, file
verification, history, watches, NAT handling and backup reception. Interfaces do
not implement their own copies of these rules.

| Location | Responsibility |
| --- | --- |
| `main.go`, `internal/cli` | Signals, flags and the original headless commands |
| `engine` | Shared protocol, persistence and client lifecycle |
| `engine/api.go`, `status.go` | Small config/control/status API, free of UI types |
| `engine/backup*.go` | Outbound camera uploads, chunk checkpoints, receiver commit |
| `engine/VERSION` | Single version source for CLI, GUI, Go mobile library and APK |
| `desktop` | Fyne widgets and one controller owning one engine lifetime |
| `mobile` | Small gomobile-compatible wrapper; native descriptors and cancellation |
| `phoneui` | Fyne phone screens and a narrow platform bridge |
| `android` | Native grants, QR scanning, scheduling, receipts and private UI service |

The root, desktop, phoneui and mobile have separate Go modules. The headless build stays
vendored and free of Fyne/cgo/Android SDK dependencies. Desktop depends on the
root module through a local replacement; mobile does the same and pins its Go
binding tools. The TOML parser version is aligned across all three builds.

## Desktop lifetime

`NewClient` opens the saved identity, folders and listener. `Client.Run(ctx)`
owns one watcher/heartbeat/cycle lifetime. The caller cancels, waits for Run to
return, then closes the client. The desktop controller prevents overlapping
starts and waits for locks to close before allowing another start.

The CLI uses the same path. There is no subprocess output scraper or GUI-only
sync implementation. The GUI deliberately owns its engine instead of attaching
to an existing CLI service through an extra privileged control protocol.

Status snapshots never block on a folder being hashed. Counts are maintained
when history is checkpointed; transfers use atomic counters. A bounded event
ring prevents activity logs from growing forever. Background goroutines schedule
widget updates with `fyne.Do`; engine code never calls the UI.

## Android lifetime

Android's WorkManager owns scheduling. Each job opens an immutable Go session
from the private TOML, then enumerates the selected granted URI tree. Local file
descriptors cross the binding; content URIs and Android framework objects do not.
Non-seekable providers use a single staging copy. Only the small binding surface
is exposed to Kotlin, keeping cryptography and wire behavior in Go.

One process-local/file lock prevents periodic and manual jobs from duplicating
work concurrently. Cancellation propagates to the Go context and any open native
source stream. Jobs are bounded; large transfers continue through chunk offsets.
No Fyne Activity or native UI lifetime is used as a background service.

SQLite stores completed receipts, not source bytes. Unknown source timestamps or
sizes are not reused as a permanent unchanged-file cache. A forced recheck tracks
its work ID so interruption does not repeatedly re-verify the first files.

## Camera upload protocol

`GET` / `PUT /v1/backup` are endpoints on receiving clients, never discovery.
Reception requires the existing folder HMAC authorization and explicit
`receive_backups`. Every request signs the method and complete query, including
the source identity, filename, full hash, total size and chunk metadata. Pinned
TLS uses the same signed discovery announcements as desktop sync.

Each PUT verifies at most 1 MiB before appending to a private partial file. A
reference-counted per-upload lock serializes a file, while the client-wide slot
limit bounds concurrent requests. Chunk checkpoints are flushed before offsets
are acknowledged. Whole-file SHA-256 verification precedes exclusive installation
and durable history. Retries acknowledge an existing file only after verification.
Edited destination files are preserved, not replaced by the backup API.

Partials live under `.douchesync/incoming`, outside manifests and watcher input.
Their count and age are bounded. Completed files become ordinary local versions
in the existing version-1 history; no state migration or identity reset is needed.
The discovery server receives neither chunk metadata nor file contents.

## Checks

```sh
make check
make test-interfaces
cd android && ./gradlew :app:testDebugUnitTest :app:assembleDebug
```

CI runs the existing CLI suite/process smoke test, desktop controller/render
checks and native builds on Linux/Windows, Android unit tests/APK builds, and
native JNI/pinned peer transfers on an Android emulator. Physical phone background
policies remain a separate field-test responsibility.

Format Go with gofmt. Kotlin sources use ktfmt's Kotlin style (0.64). Build tool
and library versions are pinned; upgrade them in their owning module, then run
the corresponding checks instead of coupling every platform's dependency graph.

## Android setup and UI state

The shared engine owns the bounded, versioned `DOUCHESYNC-CAMERA-1:` setup format and validates it using the same configuration rules as TOML. The desktop QR selects one opted-in backup folder. The mobile binding returns validated connection fields; Fyne handles review; Kotlin handles camera scanning and document-tree permissions. Desktop IDs, local paths and unrelated secrets never enter the QR. This is a reusable credential transfer, not a pairing handshake.

Android observes WorkManager for queued/running state instead of polling. Its bounded SQLite activity journal and preference-backed current snapshot are separate from durable upload receipts. Snapshot writes are scoped to destination credentials, device and source URI so cancelled workers from an old configuration cannot overwrite new destination status. UI refreshes are coalesced while active; there is no idle refresh timer.

## Fyne Android process boundary

`FyneActivity` is a NativeActivity in `:ui`. It loads only `libdouchesyncui.so`; `DoucheSyncApplication` deliberately does not load the gomobile AAR there. The default app process owns `UiProvider`, WorkManager, TOML configuration, folder grants and receipt databases and loads only the worker Go library. Do not call `Mobile.*` from the Fyne Activity or move the provider into `:ui`: the separate Go entry points must remain isolated.

The bridge uses private same-UID ContentProvider calls and standard UTF-8 byte arrays over JNI. Saved settings are validated by the existing Go binding; the Fyne form never writes a second config. Content notifications are observed only while the UI is resumed and coalesced before updating Fyne widgets. `SetupActivity` holds Android ActivityResult contracts; scanned QR values remain pending until a Fyne confirmation and explicit save. There is no local TCP control server, credential-bearing deep link or foreground-only backup loop.
