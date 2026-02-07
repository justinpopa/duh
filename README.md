# duh

**Dogmatic Unattended Hydration** — bare metal provisioning server. Single binary, zero dependencies.

duh manages PXE/HTTP boot, image hosting, and config templating for network-based OS installs. It runs on a Raspberry Pi, a VM, or anywhere you can run a Go binary.

## Features

- **PXE + HTTP boot** — serves iPXE binaries via TFTP and HTTP, supports UEFI (x86_64, ARM64) and legacy BIOS
- **Proxy DHCP** — no DHCP server changes needed on the local subnet
- **Image management** — upload or pull from a catalog; supports Linux, Windows (wimboot), ESXi, ISO, and custom iPXE scripts
- **Profile templates** — Go-templated preseed/kickstart/autoinstall configs with per-system variables
- **Webhooks** — get notified on system state changes (discovered, queued, provisioning, ready, failed)
- **TLS** — auto-generated self-signed certs, bring-your-own, or ACME/Let's Encrypt via Route53
- **Single binary** — all assets (web UI, iPXE binaries, templates) embedded via `go:embed`
- **SQLite database** — no external database required

## Quick Start

### Binary

Download from [GitHub Releases](https://github.com/justinpopa/duh/releases), then:

```bash
sudo duh --proxy-dhcp
```

This starts duh with:
- HTTP on `:80`, HTTPS on `:443`, TFTP on `:69`
- Proxy DHCP enabled (auto-detects network interface)
- Data stored in `./data/`

Open `http://<server-ip>` to access the web UI.

### Docker

```bash
docker run -d --name duh --network host \
  -v duh-data:/data \
  ghcr.io/justinpopa/duh:latest \
  --data-dir /data --proxy-dhcp
```

Or with docker-compose:

```yaml
services:
  duh:
    image: ghcr.io/justinpopa/duh:latest
    network_mode: host
    restart: unless-stopped
    volumes:
      - duh-data:/data
    command: ["--data-dir", "/data", "--proxy-dhcp"]

volumes:
  duh-data:
```

> Host networking is required for TFTP (UDP/69) and proxy DHCP (UDP/67-68).

### From Source

```bash
git clone https://github.com/justinpopa/duh.git
cd duh
make build
sudo ./bin/duh --proxy-dhcp
```

## Configuration

All options can be set via CLI flags or environment variables. Flags take precedence.

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-data-dir` | `DUH_DATA_DIR` | `./data` | Data directory |
| `-http-addr` | `DUH_HTTP_ADDR` | `:8080` | HTTP listen address |
| `-https-addr` | `DUH_HTTPS_ADDR` | `:8443` | HTTPS listen address |
| `-tftp-addr` | `DUH_TFTP_ADDR` | `:69` | TFTP listen address |
| `-server-url` | `DUH_SERVER_URL` | (auto-detect) | Server URL for boot scripts |
| `-proxy-dhcp` | `DUH_PROXY_DHCP` | `false` | Enable proxy DHCP |
| `-dhcp-iface` | `DUH_DHCP_IFACE` | (auto-detect) | Network interface for proxy DHCP |
| `-catalog-url` | `DUH_CATALOG_URL` | (built-in) | Image catalog URL |
| `-tls-cert` | `DUH_TLS_CERT` | (auto-generate) | TLS certificate file |
| `-tls-key` | `DUH_TLS_KEY` | (auto-generate) | TLS key file |
| `-acme-domain` | `DUH_ACME_DOMAIN` | | ACME/Let's Encrypt domain |
| `-acme-email` | `DUH_ACME_EMAIL` | | ACME account email |
| `-acme-staging` | `DUH_ACME_STAGING` | `false` | Use Let's Encrypt staging CA |
| `-https-redirect` | `DUH_HTTPS_REDIRECT` | `false` | Redirect HTTP to HTTPS |

### Systemd

A systemd service file is included in `deploy/`. Configuration goes in `/etc/duh/duh.env`:

```bash
DUH_DATA_DIR=/var/lib/duh
DUH_HTTP_ADDR=:80
DUH_HTTPS_ADDR=:443
DUH_TFTP_ADDR=:69
DUH_PROXY_DHCP=1
```

## How It Works

1. Client firmware sends a DHCP request
2. Proxy DHCP (or your DHCP server) responds with the duh server address and an iPXE boot filename
3. Client downloads the iPXE binary via TFTP or HTTP
4. iPXE fetches the boot script from `http://<server>/boot.ipxe`
5. The boot script loads the kernel, initrd, and config for the assigned image/profile
6. The OS installer runs using the templated config (preseed, kickstart, etc.)
7. A post-install callback notifies duh that provisioning is complete

See the **Setup** page in the web UI for DHCP configuration examples (ISC DHCP, dnsmasq, Kea, MikroTik, UniFi).

## License

[MIT](LICENSE)
