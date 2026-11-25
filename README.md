# Emser Therme Capacity Exporter

Prometheus exporter to get the capacity of the Emser Therme as metric.

## Metrics

| Name | Type | Description |
| --- | --- | --- |
| `emser_therme_capacity_percentage` | gauge | Current percentage of capacity (absent if the last scrape failed) |
| `emser_therme_scrape_success` | gauge | `1` if the last scrape succeeded, `0` otherwise |
| `emser_therme_scrape_duration_seconds` | summary | Duration of scraping the website |

Metrics are served on `/metrics`, the Go runtime and process metrics are included.

## Usage

The exporter is published as a minimal multi-arch (`linux/amd64`, `linux/arm64`) image containing only the binary:

```sh
docker run -d --name emser-therme-exporter -p 8838:8838 \
  ghcr.io/jan-di/emser-therme-capacity-exporter:edge
curl localhost:8838/metrics
```

Use a version tag (e.g. `1.2.3`) instead of `edge` for stable deployments.

Docker Compose:

```yaml
services:
  emser-therme-exporter:
    image: ghcr.io/jan-di/emser-therme-capacity-exporter:edge
    ports:
      - "8838:8838"
    restart: unless-stopped
```

### Configuration

Flags are passed as container arguments, e.g. `docker run ... <image> --interval 30s`.

| Flag | Default | Description |
| --- | --- | --- |
| `--port` | `8838` | Port to listen on |
| `--interval` | `60s` | Scraping interval |
| `--url` | `https://www.emser-therme.de` | URL of the Emser Therme website |
| `--auth-token-file` | – | File containing a bearer token required for `/metrics` |

| Environment variable | Description |
| --- | --- |
| `EXPORTER_AUTH_TOKEN` | Bearer token required for `/metrics` (ignored if `--auth-token-file` is set) |

### Authentication

By default `/metrics` is open. To require a bearer token, mount a file containing it and pass `--auth-token-file` (preferred, keeps the secret out of the environment), or set `EXPORTER_AUTH_TOKEN`:

```sh
docker run -d -p 8838:8838 -v ./token:/token:ro \
  ghcr.io/jan-di/emser-therme-capacity-exporter:edge --auth-token-file /token
```

Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: emser-therme
    authorization:
      credentials_file: /etc/prometheus/emser-token
    static_configs:
      - targets: ["emser-therme-exporter:8838"]
```

### Health check

The image has a built-in `HEALTHCHECK` (`/exporter -healthcheck`) that probes `/healthz`. The endpoint is always unauthenticated and only reports that the exporter is running, not whether the website scrape works; use `emser_therme_scrape_success` for that. If you change `--port`, override the healthcheck accordingly.

### Hardening

The image runs as an unprivileged user and contains no shell. It writes nothing to disk, so it can run locked down:

```sh
docker run -d -p 8838:8838 --read-only --cap-drop=ALL \
  --security-opt=no-new-privileges ghcr.io/jan-di/emser-therme-capacity-exporter:edge
```

## Development

Requires Go (see `go.mod` for the version).

### Run locally

```sh
go run . --port 8838 --interval 60s --url https://www.emser-therme.de
curl localhost:8838/metrics
```

All flags are optional; the values above are the defaults.

### Tests

```sh
go test ./...
```

### Build the Docker image

```sh
docker build -t emser-therme-capacity-exporter .
docker run --rm -p 8838:8838 emser-therme-capacity-exporter
```

## Credits

Inspired by [veritatron/emser-therme-widget](https://github.com/veritatron/emser-therme-widget)
