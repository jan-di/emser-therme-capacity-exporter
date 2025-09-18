# --- Build Stage ---
FROM python:3.13-slim AS builder

WORKDIR /app

RUN pip install --no-cache-dir poetry

COPY pyproject.toml poetry.lock poetry.toml ./

RUN poetry install --no-interaction --no-ansi --no-root --with main

COPY . .

RUN poetry install --no-interaction --no-ansi --with main

FROM python:3.13-slim AS runtime

WORKDIR /app

RUN useradd --create-home exporter

COPY --from=builder /app /app
RUN chown -R exporter:exporter /app

USER exporter

ENTRYPOINT ["/app/.venv/bin/python3", "-m", "emser_therme_capacity_exporter.main"]