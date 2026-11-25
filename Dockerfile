# Build natively on the CI host and cross-compile for the target platform,
# so multi-arch builds need no QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.27 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags="-s -w" -o /exporter .

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
