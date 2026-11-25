FROM golang:1.27 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /exporter .

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /exporter /exporter
# Unprivileged user; no shell, package manager or other binaries in the image.
USER 65534:65534
EXPOSE 8838
# Uses the exporter binary itself because scratch has no curl/wget.
# If you change --port, override the healthcheck accordingly.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --start-interval=1s --retries=3 \
  CMD ["/exporter", "-healthcheck"]
ENTRYPOINT ["/exporter"]
