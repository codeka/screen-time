# Deploying Screen Time Server

The repository includes a helper script `deploy.py` that packages the Go server binary and the built Angular web UI into a zip archive and copies it to a remote Linux server via `scp`.

## Prerequisites
- **Go** toolchain installed and available in `PATH` (for `go build`).
- **Node.js** and **npm** installed (for `npm ci` and `npm run build`).
- **SSH** access to the target server with a public key already configured (no password prompt).
- **Python 3** (the script uses only the standard library).

## Usage
```bash
python deploy.py --server=USER@HOST
```
Replace `USER` with the remote username (e.g., `web`) and `HOST` with the hostname or IP of your server (e.g., `screentime.codeka.com`).

The script will:
1. Build the Go server binary (`screentime-server`).
2. Install npm dependencies and build the Angular web UI (`npm run build`).
3. Package the binary, the compiled web assets, and optional `README.md`/`LICENSE` files into `screentime_deploy.zip`.
4. Transfer the zip file to `/home/USER/` on the remote server using `scp`.

If any step fails, the script aborts and prints an error message.

## Example
```bash
python deploy.py --server=web@screentime.codeka.com
```
This will place `screentime_deploy.zip` in the remote server's `/home/web/` directory.

