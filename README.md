# SGN Web Interface

A browser-based frontend for [Shikata Ga Nai](https://github.com/EgeBalci/sgn) — the polymorphic x64 shellcode encoder. Upload a binary payload, configure encoding options, and download the encoded output.

---

## Features

- Drag-and-drop payload upload
- All SGN flags exposed: encoding count, obfuscation max, bad-chars, safe registers, plain decoder, ASCII mode
- Live hex preview of encoded output (first 256 bytes)
- One-click binary download
- Dark terminal UI

---

## Local Docker

```bash
docker build -t sgn-web .
docker run -p 8080:8080 sgn-web
# → open http://localhost:8080
```

> **Note:** The build compiles keystone from source (~10 min on first run). Subsequent builds use Docker layer cache.

---

## Deploy to Railway

1. Push this folder to a GitHub repo
2. Go to [railway.app](https://railway.app) → **New Project → Deploy from GitHub repo**
3. Select the repo — Railway auto-detects the `Dockerfile`
4. Railway sets `$PORT` automatically — no config needed
5. Click **Deploy**

---

## Deploy to Render

1. Push to GitHub
2. Go to [render.com](https://render.com) → **New → Web Service**
3. Connect your repo
4. Set **Runtime** to `Docker`
5. Render injects `$PORT` automatically
6. Click **Create Web Service**

---

## Deploy to Fly.io

```bash
# Install flyctl if needed: https://fly.io/docs/hands-on/install-flyctl/
fly auth login
fly launch          # detects Dockerfile, prompts for app name & region
fly deploy
fly open            # opens your app URL
```

Fly sets `$PORT` (typically 8080) automatically via the internal proxy.

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port (set automatically by all platforms) |
| `SGN_TIMEOUT` | `5m` | Max encoding duration (Go duration string, e.g. `15m`, `1h`). Raise this if using ASCII mode. |

---

## Architecture

```
Browser
  └─► POST /encode  (multipart: payload + options)
        └─► Go web server (main.go)
              └─► exec /usr/local/bin/sgn -a 64 -i ... -o ... [flags]
                    └─► returns JSON { success, log, file (base64), inSize, outSize }
Browser downloads the decoded base64 as a binary file
```

---

## Security note

This tool is intended for **authorized penetration testing and security research** only. Do not expose it to the public internet without authentication.
