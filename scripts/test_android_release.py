# SPDX-License-Identifier: GPL-3.0-or-later
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("release", Path(__file__).with_name("verify-android-release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
CERT = "ab" * 32
OUTPUT = f"Signer #1 certificate SHA-256 digest: {CERT}\n"


class SigningTests(unittest.TestCase):
    def test_expected_certificate_allows_keytool_format(self):
        release.check_certificate(OUTPUT, ":".join(["AB"] * 32))

    def test_wrong_missing_malformed_or_multiple_signers_rejected(self):
        for output, expected in [(OUTPUT, "cd" * 32), (OUTPUT, ""), (OUTPUT, "bad"),
                                 ("", CERT), (OUTPUT + OUTPUT.replace("#1", "#2"), CERT)]:
            with self.subTest(output=output, expected=expected), self.assertRaises(ValueError):
                release.check_certificate(output, expected)

    @patch.object(release.subprocess, "run")
    def test_checksum_only_after_signature_and_certificate_verify(self, run):
        with tempfile.TemporaryDirectory() as tmp:
            apk = Path(tmp) / "release.apk"
            apk.write_bytes(b"fixture")
            checksum = apk.with_suffix(".apk.sha256")
            run.side_effect = subprocess.CalledProcessError(1, "apksigner")
            with self.assertRaises(subprocess.CalledProcessError):
                release.verify(apk, "apksigner", CERT)
            self.assertFalse(checksum.exists())
            run.side_effect = None
            run.return_value = subprocess.CompletedProcess([], 0, stdout=OUTPUT)
            with self.assertRaises(ValueError):
                release.verify(apk, "apksigner", "cd" * 32)
            self.assertFalse(checksum.exists())
            release.verify(apk, "apksigner", CERT)
            self.assertEqual(checksum.read_text(), release.hashlib.sha256(b"fixture").hexdigest() + "  release.apk\n")
            self.assertTrue(run.call_args.kwargs["check"])


if __name__ == "__main__":
    unittest.main()
