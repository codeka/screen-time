# Deploying Screen Time Server

The repository includes a helper script `deploy.py` that packages the Go server binary and the built Angular web UI into a zip archive and copies it to a remote Linux server via `scp`.

## Prerequisites
- **Go** toolchain installed and available in `PATH` (for `go build`).
- **Node.js** and **npm** installed (for `npm ci` and `npm run build`).
- **SSH** access to the target server with a public key already configured (no password prompt).
- **Python 3** (the script uses only the standard library).

## Usage
```bash
# Deploy server + web + existing Windows installer/update manifest (if present in releases/)
python deploy.py --server=USER@HOST

# Rebuild the Windows installer (and windows-latest.json) first, then deploy everything
python deploy.py --server=USER@HOST --build-windows

# Fast server/web-only deploy (skip bundling the ~43 MB Windows installer)
python deploy.py --server=USER@HOST --skip-windows
```
Replace `USER` with the remote username (e.g., `web`) and `HOST` with the hostname or IP of your server (e.g., `screentime.codeka.com`).

The script will:
1. Optionally build the Windows installer and `windows-latest.json` (`--build-windows`), using `<Version>` from `client/windows/ScreenTimeClient.csproj`.
2. Build the Go server binary (`screentime-server`).
3. Install npm dependencies and build the Angular web UI (`npm run build`).
4. Package the binary, compiled web assets, `downloads/ScreenTimeSetup.exe` + `downloads/windows-latest.json` (unless `--skip-windows`), and optional `README.md`/`LICENSE` files into `screentime_deploy.zip`.
5. Transfer the zip file to `/home/USER/` on the remote server using `scp`.

## Shipping a Windows Client Auto-Update
1. Bump `<Version>` (e.g., `1.0.1`) in `client/windows/ScreenTimeClient.csproj`.
2. Run `python deploy.py --server=web@screentime.codeka.com --build-windows` and unpack on the server so `downloads/ScreenTimeSetup.exe` and `downloads/windows-latest.json` sit next to `screentime-server` (or point `-downloads` / `DOWNLOADS_DIR` to that folder).
3. Installed Windows clients automatically check `/api/updates/windows` on startup, session unlock, and every 4 hours, verify the SHA-256 hash, silently upgrade without UAC prompts, and relaunch.

If any step fails, the script aborts and prints an error message.

## Example
```bash
python deploy.py --server=web@screentime.codeka.com
```
This will place `screentime_deploy.zip` in the remote server's `/home/web/` directory.

