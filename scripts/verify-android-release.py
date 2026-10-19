#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Verify the APK signature and pinned certificate before publishing it."""
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys


def check_certificate(output, expected):
    expected = expected.strip().replace(":", "").lower()
    if not re.fullmatch(r"[0-9a-f]{64}", expected):
        raise ValueError("Set ANDROID_SIGNING_CERT_SHA256 to the release certificate SHA-256 fingerprint")
    signers = re.findall(r"^Signer #\d+ certificate SHA-256 digest: ([0-9a-fA-F]+)$", output, re.M)
    if len(signers) != 1 or signers[0].lower() != expected:
        raise ValueError("APK signing certificate differs from ANDROID_SIGNING_CERT_SHA256; refusing publication")


def verify(apk, apksigner, expected):
    # A matching certificate alone is insufficient: the complete APK must verify.
    result = subprocess.run(
        [apksigner, "verify", "--verbose", "--print-certs", str(apk)],
        check=True, capture_output=True, text=True,
    )
    check_certificate(result.stdout, expected)
    digest = hashlib.sha256(apk.read_bytes()).hexdigest()
    apk.with_suffix(apk.suffix + ".sha256").write_text(f"{digest}  {apk.name}\n")
    print(f"Verified release APK and pinned certificate: {apk.name}")


if __name__ == "__main__":
    try:
        verify(Path(sys.argv[1]), sys.argv[2], os.environ.get("ANDROID_SIGNING_CERT_SHA256", ""))
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
