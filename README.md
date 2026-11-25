# Emser Therme Capacity Exporter

Prometheus exporter to get the capacity of the Emser Therme as metric.

## Metrics

| Name | Type | Description |
| --- | --- | --- |
| `emser_therme_capacity_percentage` | gauge | Current percentage of capacity (absent if the last scrape failed) |
| `emser_therme_scrape_success` | gauge | `1` if the last scrape succeeded, `0` otherwise |
| `emser_therme_scrape_duration_seconds` | summary | Duration of scraping the website |

## Run locally

Requires Go (see `go.mod` for the version).

```sh
go run . --port 8838 --interval 60s --url https://www.emser-therme.de
curl localhost:8838/metrics
```

All flags are optional; the values above are the defaults.

### Docker

```sh
docker build -t emser-therme-capacity-exporter .
docker run --rm -p 8838:8838 emser-therme-capacity-exporter
```

## Authentication

By default `/metrics` is open. To require a bearer token, set `EXPORTER_AUTH_TOKEN` or (preferred, keeps it out of the environment) point `--auth-token-file` at a file containing it:

```sh
docker run --rm -p 8838:8838 -v ./token:/token:ro \
  emser-therme-capacity-exporter --auth-token-file /token
```

Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: emser-therme
    authorization:
      credentials_file: /etc/prometheus/emser-token
    static_configs:
      - targets: ["exporter:8838"]
```

`/healthz` is always unauthenticated and is used by the container `HEALTHCHECK`
(`/exporter -healthcheck`). If you change `--port`, adjust the healthcheck too.

## Tests

```sh
go test ./...
```

## Credits

Inspired by [veritatron/emser-therme-widget](https://github.com/veritatron/emser-therme-widget)
