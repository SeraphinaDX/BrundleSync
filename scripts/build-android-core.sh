#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
GO_BIN=${GO:-go}
SDK=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
if [[ -z "$SDK" ]]; then echo 'Set ANDROID_HOME to your Android SDK directory.' >&2; exit 1; fi
export ANDROID_HOME="$SDK"
export ANDROID_NDK_HOME=${ANDROID_NDK_HOME:-"$SDK/ndk/28.2.13676358"}
if [[ ! -d "$ANDROID_NDK_HOME" ]]; then echo 'Install NDK 28.2.13676358, or set ANDROID_NDK_HOME.' >&2; exit 1; fi
mkdir -p "$ROOT/.build/mobile-tools" "$ROOT/android/app/libs"
cd "$ROOT/mobile"
"$GO_BIN" build -o "$ROOT/.build/mobile-tools/gomobile" golang.org/x/mobile/cmd/gomobile
"$GO_BIN" build -o "$ROOT/.build/mobile-tools/gobind" golang.org/x/mobile/cmd/gobind
export PATH="$ROOT/.build/mobile-tools:$(dirname -- "$(command -v "$GO_BIN")"):$PATH"
gomobile bind -trimpath -target=android/arm,android/arm64,android/amd64 -androidapi=24 \
  -javapkg=ca.cerberusgames.douchesync -o "$ROOT/android/app/libs/douchesync.aar" .
