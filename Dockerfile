FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /duh ./cmd/duh

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /duh /usr/local/bin/duh
VOLUME /data
EXPOSE 69/udp 8080 8443
ENTRYPOINT ["duh"]
CMD ["--data-dir", "/data"]
