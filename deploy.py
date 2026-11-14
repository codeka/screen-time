#!/usr/bin/env python3
"""Deploy script for Screen Time server.

Usage:
    python deploy.py --server=USER@HOST
    python deploy.py --server=USER@HOST --build-windows
    python deploy.py --server=USER@HOST --skip-windows

The script will:
  1. Optionally rebuild the Windows installer + update manifest (--build-windows).
  2. Build the Go server binary (Linux amd64).
  3. Build the Angular web UI (npm run build).
  4. Package the binary, built web assets, and Windows auto-update files
     (ScreenTimeSetup.exe + windows-latest.json, unless --skip-windows) into a zip archive.
  5. Copy the zip archive to `/home/USER/` on the remote server via `scp`.

Prerequisites:
  * Go toolchain in PATH (`go build`).
  * Node.js and npm installed (`npm run build`).
  * SSH access to the remote server (public key already configured).

The script is deliberately lightweight and uses only the Python standard
library. It exits with a non‑zero status on failure and prints helpful
messages.
"""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

def run_cmd(command: list, cwd: Path | None = None, description: str = "", env=None):
    """Run a command, printing its output and raising on error."""
    print(f"[+] {description}: {' '.join(command)}")
    result = subprocess.run(
        command,
        cwd=str(cwd) if cwd else None,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        encoding='utf-8',
        errors='replace',
        env=env
    )
    print(result.stdout)
    if result.returncode != 0:
        print(f"[!] {description} failed with exit code {result.returncode}")
        sys.exit(result.returncode)
    return result

def main():
    parser = argparse.ArgumentParser(description="Package and SCP deploy the Screen Time server.")
    parser.add_argument(
        "--server",
        required=True,
        help="Remote SSH destination in the form user@host (e.g., web@screentime.codeka.com)",
    )
    parser.add_argument(
        "--build-windows",
        action="store_true",
        help="Rebuild the Windows client installer and windows-latest.json before packaging",
    )
    parser.add_argument(
        "--skip-windows",
        action="store_true",
        help="Omit ScreenTimeSetup.exe and windows-latest.json from the deployment archive",
    )
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parent
    server_dir = repo_root / "server"
    web_dir = repo_root / "web"
    releases_dir = repo_root / "releases"

    if args.build_windows:
        run_cmd(
            [sys.executable, str(repo_root / "build_clients.py"), "--windows"],
            cwd=repo_root,
            description="Building Windows installer and update manifest",
        )

    # 1. Build Go server binary
    binary_path = server_dir / "screentime-server"
    if binary_path.exists():
        binary_path.unlink()
    env = os.environ.copy()
    # Cross‑compile for Linux (amd64). Adjust GOARCH if you need a different architecture.
    env["GOOS"] = "linux"
    env["GOARCH"] = "amd64"
    # Disable CGO for a fully static binary (optional but simplifies cross‑compiling).
    env["CGO_ENABLED"] = "0"
    run_cmd(["go", "build", "-o", str(binary_path)], cwd=server_dir, description="Building Go server (Linux target)", env=env)

    # 2. Build Angular web UI
    # Ensure dependencies are installed (optional)
    npm_cmd = shutil.which("npm") or "npm"
    run_cmd([npm_cmd, "ci"], cwd=web_dir, description="Installing npm dependencies")
    run_cmd([npm_cmd, "run", "build"], cwd=web_dir, description="Building Angular web UI")

    # The Angular build output is expected at web/dist/web
    build_output = web_dir / "dist" / "web"
    if not build_output.is_dir():
        print(f"[!] Expected web build output not found at {build_output}")
        sys.exit(1)

    # 3. Create temporary packaging directory
    with tempfile.TemporaryDirectory() as tmpdir:
        pkg_root = Path(tmpdir)
        # Copy binary
        shutil.copy2(binary_path, pkg_root / binary_path.name)
        # Copy web assets
        shutil.copytree(build_output, pkg_root / "web")
        # Copy Windows installer + auto-update manifest if available and not skipped
        if not args.skip_windows:
            setup_exe = releases_dir / "ScreenTimeSetup.exe"
            manifest_json = releases_dir / "windows-latest.json"
            if setup_exe.is_file() and manifest_json.is_file():
                dl_dir = pkg_root / "downloads"
                dl_dir.mkdir(exist_ok=True)
                shutil.copy2(setup_exe, dl_dir / "ScreenTimeSetup.exe")
                shutil.copy2(manifest_json, dl_dir / "windows-latest.json")
                print("[+] Bundled Windows auto-update package into downloads/")
            else:
                print("[!] Note: releases/ScreenTimeSetup.exe or windows-latest.json not found; skipping downloads/ bundle (pass --build-windows to build it).")
        # Optionally include README and LICENSE if they exist
        for extra in ["README.md", "LICENSE"]:
            src = repo_root / extra
            if src.is_file():
                shutil.copy2(src, pkg_root / extra)
        # Create zip archive
        archive_name = "screentime_deploy"
        archive_path = shutil.make_archive(base_name=str(repo_root / archive_name), format="zip", root_dir=str(pkg_root))
        print(f"[+] Created archive {archive_path}")

        # 4. SCP to remote server (to /home/<user>/)
        remote_path = f"{args.server}:/home/{args.server.split('@')[0]}/"
        run_cmd(["scp", archive_path, remote_path], description="Transferring archive via scp")
        print("[+] Deployment package transferred successfully.")

if __name__ == "__main__":
    main()

