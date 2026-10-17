#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Reuse an installed SDK, or bootstrap pinned official tools on a slim CI image.
set -euo pipefail
SDK=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-"$RUNNER_TEMP/douchesync-android-sdk"}}
SDKMANAGER="$SDK/cmdline-tools/latest/bin/sdkmanager"
if [[ ! -x "$SDKMANAGER" ]]; then
  archive="$RUNNER_TEMP/android-commandlinetools.zip"
  curl -fsSL https://dl.google.com/android/repository/commandlinetools-linux-13114758_latest.zip -o "$archive"
  printf '%s  %s\n' '7ec965280a073311c339e571cd5de778b9975026cfcbe79f2b1cdcb1e15317ee' "$archive" | sha256sum --check -
  mkdir -p "$SDK/cmdline-tools"
  unzip -q "$archive" -d "$SDK/cmdline-tools"
  mv "$SDK/cmdline-tools/cmdline-tools" "$SDK/cmdline-tools/latest"
fi
export ANDROID_HOME="$SDK"
echo "ANDROID_HOME=$SDK" >> "$GITHUB_ENV"
printf '%s\n' "$SDK/cmdline-tools/latest/bin" "$SDK/platform-tools" "$SDK/emulator" >> "$GITHUB_PATH"
# yes normally exits on SIGPIPE after sdkmanager closes stdin. Preserve the SDK
# command's exit status without mistaking that expected pipe exit for failure.
set +o pipefail
yes | "$SDKMANAGER" --sdk_root="$SDK" --licenses >/dev/null
license_result=${PIPESTATUS[1]}
set -o pipefail
if [[ "$license_result" != 0 ]]; then exit "$license_result"; fi
"$SDKMANAGER" --sdk_root="$SDK" 'platforms;android-36' 'build-tools;35.0.0' 'ndk;28.2.13676358' emulator 'system-images;android-35;google_apis;x86_64'
