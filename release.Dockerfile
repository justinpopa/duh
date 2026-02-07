FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY duh /usr/local/bin/duh
VOLUME /data
EXPOSE 69/udp 8080 8443
ENTRYPOINT ["duh"]
CMD ["--data-dir", "/data"]
