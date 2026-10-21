# Stable Android releases and Obtainium

Android updates must use the same application ID and signing certificate as the
installed app. The old CI artifacts used a newly generated debug keystore on each
runner. Attaching those APKs to releases caused Obtainium certificate mismatches.

Starting with 0.7.2, use the **Signed Android release** workflow for release APKs.
It uses one persistent key, verifies the APK and a separately pinned certificate
fingerprint, then attaches `BrundleSync-v<VERSION>.apk` and its SHA-256 checksum to
the GitHub release. Missing credentials or a certificate mismatch stop publication.
The Desktop and Android workflow's debug APK remains for testing only: never
attach it to a release used by Obtainium.

BrundleSync uses the same signing key as earlier releases. If signing is already
configured, skip key generation and keep the existing secrets, alias and pinned
fingerprint. See [upgrading after the rename](RENAMING.md).

## One-time maintainer setup

Do this on your own computer. Do not commit the keystore, paste passwords into
issues, or generate a fresh key for each release. If you already have the private
key that signed an installed release, reuse it instead of generating another.
An APK contains the public certificate, not the private signing key.

These commands require a JDK (`keytool`) and the GitHub CLI (`gh`), authenticated
with access to this repository's Actions secrets and variables. On Linux:

```sh
mkdir -p ~/.config/douchesync-signing
chmod 700 ~/.config/douchesync-signing
keytool -genkeypair -keystore ~/.config/douchesync-signing/release.p12 \
  -storetype PKCS12 -alias douchesync -keyalg RSA -keysize 3072 -validity 10000
chmod 600 ~/.config/douchesync-signing/release.p12
```

Enter a strong password when prompted. Keep an encrypted backup of the keystore
and its password somewhere independent of GitHub. Losing this key prevents
ordinary updates to installations signed with it.

Upload it to [repository Actions secrets](https://github.com/SeraphinaDX/DoucheSync/settings/secrets/actions):

```sh
base64 -w 0 ~/.config/douchesync-signing/release.p12 | \
  gh secret set ANDROID_KEYSTORE_BASE64 --repo SeraphinaDX/DoucheSync
gh secret set ANDROID_KEYSTORE_PASSWORD --repo SeraphinaDX/DoucheSync
gh secret set ANDROID_KEY_ALIAS --body douchesync --repo SeraphinaDX/DoucheSync
gh secret set ANDROID_KEY_PASSWORD --repo SeraphinaDX/DoucheSync
```

The password commands prompt for their values. For this PKCS12 keystore, enter
the same password for both password secrets. Base64 is only an encoding; its
output must stay in the secret, not in a committed file.

Print the public certificate fingerprint:

```sh
keytool -list -v -keystore ~/.config/douchesync-signing/release.p12 -alias douchesync
```

Copy the **SHA256** fingerprint and set a repository Actions **variable** named
`ANDROID_SIGNING_CERT_SHA256` at
[repository Actions variables](https://github.com/SeraphinaDX/DoucheSync/settings/variables/actions).
Colons and uppercase letters are accepted. This is the public certificate
fingerprint, not the APK file checksum. Keep it pinned to this key; do not change
it simply to make a mismatched build pass.

## Publish updates

1. Merge the signing workflow and set the secrets and variable above.
2. Set `engine/VERSION` to the new version and commit it. The initial transition
   release is **0.7.2**; later releases must increase the version.
3. Create and publish the GitHub release with a tag matching that commit's
   version, such as `v0.7.2`. Do not manually attach a debug APK.
4. The **Signed Android release** workflow runs automatically and attaches the
   verified APK. Wait for that workflow to succeed before updating through
   Obtainium. Publishing the release alone does not mean its APK is ready.

To retry after adding missing secrets, open **Actions → Signed Android release →
Run workflow**, select `main` and enter the existing tag. The workflow checks out
that tag, requires it to match `engine/VERSION`, and uploads to the existing
release. It refuses to overwrite existing assets. A tag predating the signing
implementation cannot be built with this workflow; create the new release from
the merged implementation.

The same four credential environment variables are accepted for local builds:
`ANDROID_KEYSTORE_PATH` (absolute keystore path), `ANDROID_KEYSTORE_PASSWORD`,
`ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`. Then run
`cd android && ./gradlew :app:assembleRelease`. The signed output is
`app/build/outputs/apk/release/app-release.apk`. Debug builds need no credentials;
release builds have no unsigned/debug fallback. For local publication, also set
`ANDROID_SIGNING_CERT_SHA256` and run `python3 scripts/verify-android-release.py` with the
APK path and the SDK's `build-tools/35.0.0/apksigner` path.

## Existing installations

A permanent key fixes future updates; it cannot make a new certificate compatible
with an already installed APK signed by a different key. If the original private
key is still available, reuse it. Otherwise the transition requires a **one-time
uninstall and install of the new signed release**. Uninstalling erases the app's
private configuration, saved folder permissions and backup receipt index. Record
your settings first, then scan the desktop QR and choose the camera folder again
after installing. A new scan may be needed to rebuild the receipt index.

Do not uninstall until the new signed APK is available. Keep Obtainium pointed at
this repository's release APK. Once the installed app uses the permanent key,
subsequent signed releases update in place and retain app data. Debug test builds
remain incompatible with a release-signed installation.
