#!/usr/bin/env python3
"""Run native Android JNI/peer tests against real discovery and desktop processes.

Requires a booted Android emulator, the CLI binary, and the Android build tools.
The test's HTTP discovery is loopback/emulator-only; peer content uses pinned TLS.
"""
import hashlib
import hmac
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def main():
    processes, handles = [], []
    with tempfile.TemporaryDirectory(prefix="douchesync-android-") as tmp:
        work = Path(tmp)
        server_dir, peer_dir = work / "discovery", work / "desktop"
        server_dir.mkdir()
        peer_dir.mkdir()
        token, secret = secrets.token_hex(32), secrets.token_hex(32)
        server_port, peer_port = port(), port()
        server = work / "server.toml"
        server.write_text(f'[server]\nlisten="127.0.0.1:{server_port}"\ntoken="{token}"\n')
        peer = work / "desktop.toml"
        peer.write_text(
            f'[client]\ndevice_id="desktop-native-test"\n'
            f'identity_dir={json.dumps(str(work / "identity"))}\n'
            f'listen="0.0.0.0:{peer_port}"\nadvertise_url="https://10.0.2.2:{peer_port}"\n'
            f'discovery_url="http://127.0.0.1:{server_port}"\ndiscovery_token="{token}"\n'
            f'allow_http_discovery=true\n[[folders]]\nid="camera"\n'
            f'path={json.dumps(str(peer_dir))}\nsecret="{secret}"\nreceive_backups=true\n')

        def launch(mode, config, cwd):
            handle = (work / f"{mode}.log").open("wb")
            handles.append(handle)
            processes.append(subprocess.Popen([str(ROOT / "DoucheSync"), mode, "-config=" + str(config)], cwd=cwd, stdout=handle, stderr=handle))

        try:
            launch("server", server, server_dir)
            time.sleep(0.3)
            launch("client", peer, work)
            room = hmac.new(secret.encode(), b"douchesync-v1-room\ncamera", hashlib.sha256).hexdigest()
            deadline = time.monotonic() + 20
            while True:
                if any(p.poll() is not None for p in processes):
                    raise RuntimeError("test discovery or desktop process exited")
                try:
                    request = urllib.request.Request(f"http://127.0.0.1:{server_port}/v1/peers?room={room}", headers={"Authorization": "Bearer " + token})
                    with urllib.request.urlopen(request, timeout=1) as response:
                        if json.load(response):
                            break
                except OSError:
                    pass
                if time.monotonic() > deadline:
                    raise RuntimeError("desktop did not register with discovery")
                time.sleep(0.2)
            command = [str(ROOT / "android/gradlew"), "--no-daemon", ":app:connectedDebugAndroidTest",
                       "-Pandroid.injected.androidTest.leaveApksInstalledAfterRun=true",
                       f"-Pandroid.testInstrumentationRunnerArguments.discovery=http://10.0.2.2:{server_port}",
                       f"-Pandroid.testInstrumentationRunnerArguments.token={token}",
                       f"-Pandroid.testInstrumentationRunnerArguments.secret={secret}"]
            subprocess.run(command, cwd=ROOT / "android", check=True, timeout=300)
            photos = list(peer_dir.rglob("native-photo-*.jpg"))
            expected = bytes(i % 251 for i in range(2 * 1024 * 1024 + 23))
            if len(photos) != 1 or photos[0].read_bytes() != expected:
                raise RuntimeError("Android peer transfer did not produce the exact original bytes")
            if list(server_dir.iterdir()):
                raise RuntimeError("discovery stored files")
            print("PASS: Android file descriptor -> chunked pinned-TLS peer upload -> verified desktop file; discovery stored no files")
        finally:
            for process in reversed(processes):
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            for handle in handles:
                handle.close()
            if any(p.returncode != 0 for p in processes):
                for log in work.glob("*.log"):
                    print(log.read_text())


if __name__ == "__main__":
    main()
