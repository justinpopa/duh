.PHONY: build run dev clean build-pi deploy

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/duh ./cmd/duh

run: build
	./bin/duh

dev: build
	./bin/duh --tftp-addr :6969 --http-addr :8080 --https-addr :8443

dev-pxe: build
	sudo -E ./bin/duh --proxy-dhcp --tftp-addr :69 --http-addr :8080 --https-addr :8443

IPXE_COMMIT := 362b704f833cb2b0d7bf77ac97b2e06298211385

ipxe: ipxe-x86 ipxe-arm64

ipxe-x86:
	docker run --rm --platform linux/amd64 -v $(CURDIR)/internal/tftpserver/ipxebin:/out alpine:3.21 sh -c '\
		apk add --no-cache gcc musl-dev make perl xz-dev mtools git cdrkit && \
		git init /build && cd /build && \
		git fetch --depth 1 https://github.com/ipxe/ipxe.git $(IPXE_COMMIT) && \
		git checkout FETCH_HEAD && \
		cd src && \
		make -j$$(nproc) bin-x86_64-efi/ipxe.efi bin/undionly.kpxe && \
		cp bin-x86_64-efi/ipxe.efi /out/ipxe.efi && \
		cp bin/undionly.kpxe /out/undionly.kpxe'

ipxe-arm64:
	docker run --rm --platform linux/arm64 -v $(CURDIR)/internal/tftpserver/ipxebin:/out alpine:3.21 sh -c '\
		apk add --no-cache gcc musl-dev make perl xz-dev mtools git cdrkit && \
		git init /build && cd /build && \
		git fetch --depth 1 https://github.com/ipxe/ipxe.git $(IPXE_COMMIT) && \
		git checkout FETCH_HEAD && \
		cd src && \
		make -j$$(nproc) bin-arm64-efi/snp.efi && \
		cp bin-arm64-efi/snp.efi /out/ipxe-arm64.efi'

clean:
	rm -rf bin/ data/

# --- RPi deployment ---
# Usage: make deploy PI=pi@192.168.1.100

build-pi:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/duh-linux-arm64 ./cmd/duh

deploy: build-pi
	@if [ -z "$(PI)" ]; then echo "Usage: make deploy PI=user@host"; exit 1; fi
	scp bin/duh-linux-arm64 $(PI):/tmp/duh
	scp deploy/duh.service $(PI):/tmp/duh.service
	scp deploy/duh.env $(PI):/tmp/duh.env
	ssh $(PI) '\
		sudo mv /tmp/duh /usr/local/bin/duh && \
		sudo chmod 755 /usr/local/bin/duh && \
		sudo mv /tmp/duh.service /etc/systemd/system/duh.service && \
		sudo mkdir -p /etc/duh && \
		[ -f /etc/duh/duh.env ] || sudo mv /tmp/duh.env /etc/duh/duh.env && \
		sudo mkdir -p /var/lib/duh && \
		sudo systemctl daemon-reload && \
		sudo systemctl enable duh && \
		sudo systemctl restart duh && \
		echo "duh deployed and running" && \
		sudo systemctl status duh --no-pager'
