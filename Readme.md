# EV Charger TOU Pricing API

Backend service for managing charger-specific Time-of-Use (TOU) pricing schedules.

## Tech Stack

- Go 1.22
- Chi (HTTP router)
- GORM (ORM)
- PostgreSQL 16
- Docker + Docker Compose
- OpenAPI 3 (`swagger.yaml`)

## Project Structure

```text
cmd/
  api/
    main.go              # service bootstrap and HTTP server
internal/
  api/
    handler.go           # HTTP handlers + validation + response mapping
    router.go            # route registration
  config/
    config.go            # environment configuration
  database/
    database.go          # DB connection + migrations
  models/
    models.go            # GORM entities
docker-compose.yml
Dockerfile
swagger.yaml
Readme.md
```

## Domain Model

- `chargers`: charger metadata (`id`, `name`, `timezone`)
- `pricing_schedules`: effective date window per charger
- `pricing_periods`: daily price windows (`start_time`, `end_time`, `price_per_kwh`) linked to a schedule

This keeps the schema normalized and query-friendly.

## Run Locally

```bash
docker compose up --build
```

Services:
- API: `http://localhost:8080`
- Postgres: `localhost:5432`

Health check:

```bash
curl http://localhost:8080/healthz
```

## API Endpoints

- `POST /api/v1/chargers` - Create charger
- `PUT /api/v1/chargers/{chargerID}/pricing` - Add pricing schedule for a charger
- `GET /api/v1/chargers/{chargerID}/pricing` - Retrieve pricing using either `date+time` or `timestamp`
- `PUT /api/v1/pricing/bulk` - Apply pricing schedule to multiple chargers

## Sample Requests

Create charger:

```bash
curl -X POST http://localhost:8080/api/v1/chargers \
  -H "Content-Type: application/json" \
  -d '{
    "id": "charger-001",
    "name": "Downtown Charger",
    "timezone": "Asia/Kolkata"
  }'
```

Update pricing for one charger:

```bash
curl -X PUT http://localhost:8080/api/v1/chargers/charger-001/pricing \
  -H "Content-Type: application/json" \
  -d '{
    "effective_from": "2026-03-08",
    "effective_to": null,
    "periods": [
      {"start_time":"00:00","end_time":"06:00","price_per_kwh":0.15},
      {"start_time":"06:00","end_time":"12:00","price_per_kwh":0.20},
      {"start_time":"12:00","end_time":"14:00","price_per_kwh":0.25},
      {"start_time":"14:00","end_time":"18:00","price_per_kwh":0.30},
      {"start_time":"18:00","end_time":"20:00","price_per_kwh":0.25},
      {"start_time":"20:00","end_time":"22:00","price_per_kwh":0.20},
      {"start_time":"22:00","end_time":"23:59","price_per_kwh":0.15}
    ]
  }'
```

Get pricing by date/time:

```bash
curl "http://localhost:8080/api/v1/chargers/charger-001/pricing?date=2026-03-08&time=14:30"
```

Get pricing by timestamp (timezone-aware):

```bash
curl "http://localhost:8080/api/v1/chargers/charger-001/pricing?timestamp=2026-03-08T09:00:00Z"
```

Bulk update:

```bash
curl -X PUT http://localhost:8080/api/v1/pricing/bulk \
  -H "Content-Type: application/json" \
  -d '{
    "charger_ids": ["charger-001", "charger-002"],
    "effective_from": "2026-03-08",
    "effective_to": null,
    "periods": [
      {"start_time":"00:00","end_time":"06:00","price_per_kwh":0.15},
      {"start_time":"06:00","end_time":"12:00","price_per_kwh":0.20},
      {"start_time":"12:00","end_time":"18:00","price_per_kwh":0.30},
      {"start_time":"18:00","end_time":"23:59","price_per_kwh":0.25}
    ]
  }'
```

## Swagger

Swagger spec lives in `swagger.yaml`.

Run Swagger UI:

```bash
docker run --rm -p 8090:8080 \
  -e SWAGGER_JSON=/spec/swagger.yaml \
  -v "$(pwd)/swagger.yaml:/spec/swagger.yaml:ro" \
  swaggerapi/swagger-ui
```

Open `http://localhost:8090`.

## Notes for Interviewers

- Charger-level pricing (not region-level) is supported.
- Timezone handling is implemented via:
  - charger timezone persistence
  - timestamp-to-local conversion for pricing lookup
- Bulk updates are supported through a dedicated endpoint.
