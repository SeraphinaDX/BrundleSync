#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Build Fyne as a NativeActivity library within the existing Gradle APK. The
# upstream Activity classes are copied from the pinned Fyne module, never patched.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
GO_BIN=${GO:-go}
SDK=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
NDK=${ANDROID_NDK_HOME:-"$SDK/ndk/28.2.13676358"}
case "$(uname -s)" in Linux) host=linux-x86_64;; Darwin) host=darwin-x86_64;; *) echo 'Build Android on Linux or macOS.' >&2; exit 1;; esac
TOOLS="$NDK/toolchains/llvm/prebuilt/$host/bin"
if [[ ! -x "$TOOLS/aarch64-linux-android24-clang" ]]; then echo 'Set ANDROID_HOME and install NDK 28.2.13676358.' >&2; exit 1; fi
cd "$ROOT/phoneui"
FYNE=$("$GO_BIN" list -m -f '{{.Dir}}' fyne.io/fyne/v2)
JAVA_OUT="$ROOT/.build/fyne-java/org/golang/app"
JNI_OUT="$ROOT/.build/fyne-jni"
mkdir -p "$JAVA_OUT" "$ROOT/.build/fyne-assets"
cp "$FYNE/internal/driver/mobile/app/GoNativeActivity.java" "$JAVA_OUT/"
cp "$FYNE/internal/driver/mobile/app/FyneNotificationReceiver.java" "$JAVA_OUT/"
cp "$FYNE/LICENSE" "$ROOT/.build/Fyne-LICENSE"
cp "$FYNE/LICENSE" "$ROOT/.build/fyne-assets/Fyne-LICENSE"
for spec in 'arm armeabi-v7a armv7a-linux-androideabi24-clang' 'arm64 arm64-v8a aarch64-linux-android24-clang' 'amd64 x86_64 x86_64-linux-android24-clang'; do
 read -r arch abi compiler <<< "$spec"
 mkdir -p "$JNI_OUT/$abi"
 CGO_ENABLED=1 GOOS=android GOARCH="$arch" GOARM=7 CC="$TOOLS/$compiler" \
   "$GO_BIN" build -trimpath -tags accessibility -buildmode=c-shared -ldflags='-w' -o "$JNI_OUT/$abi/libdouchesyncui.so" .
 # A missing entry point must fail the build rather than ship a blank Activity.
 "$TOOLS/llvm-nm" -D "$JNI_OUT/$abi/libdouchesyncui.so" | awk '{print $3}' > "$ROOT/.build/fyne-symbols-$arch.txt"
 for symbol in main.main ANativeActivity_onCreate Java_ca_cerberusgames_douchesync_FyneActivity_uiChanged; do
  if ! grep -qx "$symbol" "$ROOT/.build/fyne-symbols-$arch.txt"; then echo "Missing Fyne entry point: $symbol ($abi)" >&2; exit 1; fi
 done
done
