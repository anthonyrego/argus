# Deploying argus to an Ubuntu server

Runs argus under systemd so it survives SSH disconnects, crashes, and reboots.
The SQLite driver is pure Go, so no CGO toolchain is required on the target —
cross-compile on your dev machine and ship the binary.

## 1. Build on your dev machine

```bash
cd web && npm run build && cd ..              # embed the SPA into the binary
GOOS=linux GOARCH=amd64 go build -o argus-linux .
# or GOARCH=arm64 for a Pi / Ampere host
scp argus-linux config.yaml deploy/argus.service user@server:/tmp/
# and the APNs .p8 key, if push is enabled:
# scp keys/AuthKey_XXXX.p8 user@server:/tmp/
```

## 2. Install on the server

```bash
sudo apt install -y ffmpeg
sudo useradd --system --home /var/lib/argus --shell /usr/sbin/nologin argus
sudo mkdir -p /var/lib/argus/recordings /etc/argus
sudo mv /tmp/argus-linux /usr/local/bin/argus
sudo chmod +x /usr/local/bin/argus
sudo mv /tmp/config.yaml /etc/argus/config.yaml
sudo mv /tmp/argus.service /etc/systemd/system/argus.service
# optional, if pushing notifications:
# sudo mv /tmp/AuthKey_XXXX.p8 /etc/argus/
sudo chown -R argus:argus /var/lib/argus /etc/argus
```

Edit `/etc/argus/config.yaml` so paths are absolute under `/var/lib/argus`:

```yaml
server:
  database: "/var/lib/argus/argus.db"
recordings:
  dir: "/var/lib/argus/recordings"
apns:
  key_path: "/etc/argus/AuthKey_XXXX.p8"   # only if push is enabled
```

## 3. Start it

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now argus
journalctl -u argus -f                        # tail logs
```

The server is now on `http://<host>:8080`. Default admin login is
`admin` / `admin` — change it on first sign-in.

## HTTPS

The unit listens on plain HTTP. To get a real TLS cert without managing
one yourself, run on the same host:

```bash
sudo tailscale serve --bg --https=443 http://localhost:8080
```

The argus UI is then at `https://<host>.<tailnet>.ts.net/` over the
tailnet. The serve config persists across reboots on its own.

## Updates

```bash
# on the dev machine
cd web && npm run build && cd ..
GOOS=linux GOARCH=amd64 go build -o argus-linux .
scp argus-linux user@server:/tmp/

# on the server
sudo mv /tmp/argus-linux /usr/local/bin/argus
sudo systemctl restart argus
```
