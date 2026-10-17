#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Run on a host with working KVM; install the API 35 image first (see docs).
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
SDK=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
if [[ -z "$SDK" ]]; then echo 'Set ANDROID_HOME to the Android SDK directory.' >&2; exit 1; fi
export PATH="$SDK/platform-tools:$SDK/emulator:$SDK/cmdline-tools/latest/bin:$PATH"
export ANDROID_AVD_HOME="$ROOT/.build/avd"
export ANDROID_USER_HOME="$ROOT/.build/android-user"
mkdir -p "$ANDROID_AVD_HOME" "$ANDROID_USER_HOME"
printf 'no\n' | avdmanager create avd --force --name douchesync-test --path "$ANDROID_AVD_HOME/douchesync-test.avd" --package 'system-images;android-35;google_apis;x86_64'
emulator -list-avds
adb start-server
emulator -avd douchesync-test -no-window -no-audio -no-boot-anim -no-snapshot -gpu swiftshader -memory 2048 -cores 2 > "$ROOT/.build/emulator.log" 2>&1 &
EMU_PID=$!
trap 'kill "$EMU_PID" 2>/dev/null || true' EXIT
booted=false
for attempt in $(seq 1 120); do
  result=$(timeout 3s adb shell getprop sys.boot_completed 2>/dev/null || true)
  if [[ "${result//$'\r'/}" == 1 ]]; then booted=true; break; fi
  if ! kill -0 "$EMU_PID" 2>/dev/null; then cat "$ROOT/.build/emulator.log"; exit 1; fi
  sleep 1
done
if [[ "$booted" != true ]]; then cat "$ROOT/.build/emulator.log"; echo 'Android emulator did not boot.' >&2; exit 1; fi
adb shell input keyevent 82
python3 "$ROOT/scripts/android-integration.py"
