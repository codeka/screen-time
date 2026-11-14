#!/usr/bin/env python3
"""Build release packages for Screen Time clients (Windows installer + Android APK).

Usage:
    python build_clients.py             # Build both Windows installer and Android release APK
    python build_clients.py --windows   # Build Windows installer only
    python build_clients.py --android   # Build Android release APK only

Outputs are copied to ./releases/:
  - releases/ScreenTimeSetup.exe
  - releases/screentime-release.apk
"""

import argparse
import datetime
import hashlib
import json
import os
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET
from pathlib import Path


def run_cmd(command: list[str], cwd: Path | None = None, description: str = "", env: dict | None = None):
    print(f"[+] {description}: {' '.join(command)}")
    result = subprocess.run(
        command,
        cwd=str(cwd) if cwd else None,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    if result.stdout:
        print(result.stdout)
    if result.returncode != 0:
        print(f"[!] {description} failed with exit code {result.returncode}")
        sys.exit(result.returncode)
    return result


def find_iscc() -> str:
    found = shutil.which("iscc") or shutil.which("ISCC")
    if found:
        return found
    candidates = [
        Path(os.environ.get("LOCALAPPDATA", "")) / "Programs" / "Inno Setup 6" / "ISCC.exe",
        Path(os.environ.get("ProgramFiles(x86)", r"C:\Program Files (x86)")) / "Inno Setup 6" / "ISCC.exe",
        Path(os.environ.get("ProgramFiles", r"C:\Program Files")) / "Inno Setup 6" / "ISCC.exe",
    ]
    for candidate in candidates:
        if candidate.is_file():
            return str(candidate)
    print("[!] Could not find Inno Setup Compiler (ISCC.exe). Install via: winget install JRSoftware.InnoSetup")
    sys.exit(1)


def find_java_home() -> str | None:
    if os.environ.get("JAVA_HOME") and (Path(os.environ["JAVA_HOME"]) / "bin" / "java.exe").is_file():
        return os.environ["JAVA_HOME"]
    java_dir = Path(r"C:\Program Files\Java")
    if java_dir.is_dir():
        # Prefer JDK 21 or 17 for Android Gradle Plugin compatibility
        for pref in ["jdk-21", "jdk-17"]:
            matches = sorted(java_dir.glob(f"{pref}*"), reverse=True)
            if matches:
                return str(matches[0])
        jdks = sorted(java_dir.glob("jdk*"), reverse=True)
        if jdks:
            return str(jdks[0])
    return None


def get_windows_csproj_version(csproj_path: Path) -> str:
    tree = ET.parse(csproj_path)
    root = tree.getroot()
    for elem in root.iter("Version"):
        if elem.text and elem.text.strip():
            return elem.text.strip()
    return "1.0.0"


def compute_sha256(file_path: Path) -> str:
    h = hashlib.sha256()
    with file_path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def build_windows(repo_root: Path, releases_dir: Path):
    win_dir = repo_root / "client" / "windows"
    version = get_windows_csproj_version(win_dir / "ScreenTimeClient.csproj")
    print(f"[+] Building Windows client v{version}")

    publish_name = "publish"
    publish_dir = win_dir / publish_name
    if publish_dir.exists():
        try:
            shutil.rmtree(publish_dir)
        except PermissionError:
            publish_name = "publish_staging"
            publish_dir = win_dir / publish_name
            if publish_dir.exists():
                shutil.rmtree(publish_dir)

    run_cmd(
        ["dotnet", "publish", "-c", "Release", "-r", "win-x64", "--self-contained", "-o", str(publish_dir)],
        cwd=win_dir,
        description=f"Publishing self-contained Windows client v{version}",
    )

    iscc = find_iscc()
    run_cmd(
        [
            iscc,
            f"/DPublishDir={publish_name}",
            f"/DMyAppVersion={version}",
            str(win_dir / "installer.iss"),
        ],
        cwd=win_dir,
        description=f"Compiling Inno Setup Windows installer v{version}",
    )

    built_setup = win_dir / "installer_output" / "ScreenTimeSetup.exe"
    dest = releases_dir / "ScreenTimeSetup.exe"
    shutil.copy2(built_setup, dest)

    sha256_hex = compute_sha256(dest)
    size_bytes = dest.stat().st_size
    manifest = {
        "version": version,
        "sha256": sha256_hex,
        "url": "/downloads/ScreenTimeSetup.exe",
        "size_bytes": size_bytes,
        "published_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    manifest_path = releases_dir / "windows-latest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

    size_mb = size_bytes / (1024 * 1024)
    print(f"[+] Windows installer ready: {dest} ({size_mb:.1f} MB, sha256={sha256_hex[:16]}...)")
    print(f"[+] Windows update manifest written: {manifest_path}")


def build_android(repo_root: Path, releases_dir: Path):
    android_dir = repo_root / "client" / "android"
    gradlew = android_dir / ("gradlew.bat" if os.name == "nt" else "gradlew")

    env = os.environ.copy()
    java_home = find_java_home()
    if java_home:
        env["JAVA_HOME"] = java_home
        env["PATH"] = str(Path(java_home) / "bin") + os.pathsep + env.get("PATH", "")

    run_cmd(
        [str(gradlew), "assembleRelease"],
        cwd=android_dir,
        description="Building signed Android release APK",
        env=env,
    )

    built_apk = android_dir / "app" / "build" / "outputs" / "apk" / "release" / "app-release.apk"
    if not built_apk.is_file():
        print(f"[!] Expected APK not found at {built_apk}")
        sys.exit(1)

    dest = releases_dir / "screentime-release.apk"
    shutil.copy2(built_apk, dest)
    size_mb = dest.stat().st_size / (1024 * 1024)
    print(f"[+] Android release APK ready: {dest} ({size_mb:.1f} MB)")


def main():
    parser = argparse.ArgumentParser(description="Build Screen Time client release packages.")
    parser.add_argument("--windows", action="store_true", help="Build Windows installer only")
    parser.add_argument("--android", action="store_true", help="Build Android release APK only")
    args = parser.parse_args()

    build_both = not args.windows and not args.android

    repo_root = Path(__file__).resolve().parent
    releases_dir = repo_root / "releases"
    releases_dir.mkdir(exist_ok=True)

    if build_both or args.windows:
        build_windows(repo_root, releases_dir)
    if build_both or args.android:
        build_android(repo_root, releases_dir)


if __name__ == "__main__":
    main()

