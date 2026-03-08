# Local Setup and Swagger

## 1) Run on localhost

```bash
docker compose up --build
```

This starts:
- Postgres on `localhost:5432`
- API on `localhost:8080`

Health check:

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```

## 2) Open Swagger UI

Use Swagger UI container and mount local spec file:

```bash
docker run --rm -p 8090:8080 \
  -e SWAGGER_JSON=/spec/swagger.yaml \
  -v "$(pwd)/swagger.yaml:/spec/swagger.yaml:ro" \
  swaggerapi/swagger-ui
```

Open:
- `http://localhost:8090`

## 3) Quick API flow

### Create charger

```bash
curl -X POST http://localhost:8080/api/v1/chargers \
  -H "Content-Type: application/json" \
  -d '{
    "id": "charger-001",
    "name": "Downtown Charger",
    "timezone": "Asia/Kolkata"
  }'
```

### Add/update pricing schedule

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

### Get pricing for date/time

```bash
curl "http://localhost:8080/api/v1/chargers/charger-001/pricing?date=2026-03-08&time=14:30"
```

### Get pricing using timestamp (timezone-aware)

The API converts timestamp into charger local timezone before matching the pricing period.

```bash
curl "http://localhost:8080/api/v1/chargers/charger-001/pricing?timestamp=2026-03-08T09:00:00Z"
```

### Bulk update pricing for multiple chargers

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

## 4) Stop containers

```bash
docker compose down
```
