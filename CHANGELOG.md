# Changelog

## v0.1.0

Initial release.

- PXE boot server with TFTP and HTTP support
- UEFI (x86_64, ARM64) and legacy BIOS boot
- Proxy DHCP for zero-config network boot on local subnet
- Web UI for managing systems, images, profiles, and webhooks
- Image management: upload or pull from catalog
- Boot types: Linux, Windows (wimboot), ESXi, ISO, custom iPXE
- Profile templating with Go templates and per-system variables
- System state machine: discovered, queued, provisioning, ready, failed
- Webhooks with event filtering and HMAC signing
- TLS: auto-generated self-signed, bring-your-own, or ACME/Let's Encrypt (Route53)
- SQLite database (no external dependencies)
- Single binary with all assets embedded
- Systemd service file and Docker support
