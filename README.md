# SecArch Tickets

SecArch Tickets discovers the two supported SecArch request types from CMDB,
groups them by the owning super department, and stores a durable PostgreSQL
snapshot. A full Open + Closed synchronization runs at service startup. Later
synchronizations are started only when a user clicks Refresh; there is no timer.

## Routes

- `GET /`
- `GET /api/tickets`
- `POST /api/tickets/refresh`
- `PUT /api/tickets/:ticket_number/expected-date`
- `GET /api/tickets/:ticket_number/updates`
- `POST /api/tickets/:ticket_number/updates`
- `GET /api/statistics/closed?start=YYYY-MM-DD&end=YYYY-MM-DD`
- `GET /healthz`

## Configuration

The service uses these environment variables:

- `APP_ENV`: `qa` or `prod`; defaults to `qa`.
- `APP_CONFIG_DIR`: directory containing `db.json` and `cmdb.json`.
- `APP_TLS_DIR`: directory containing server and client certificates.
- `APP_LISTEN_ADDR`: HTTPS listen address; defaults to `:8443`.

The database configuration uses a `db_servers` array. pgx tries the servers in
order and accepts only a writable PostgreSQL session, allowing new pool
connections to follow a primary/standby failover.

The CMDB configuration contains ticket and objects endpoints, paging/batching
limits, the shared HTTP timeout, refresh cooldown/backoff values, and circuit
breaker settings. QA and production examples are under `config/`.

The service stores tickets in `secarch_tickets.tickets` within the existing
`ai_info_db01` PostgreSQL database. CMDB fields are refreshed in place and are
never hard-deleted by this service. Expected dates and local progress notes are
maintained locally and are never written back to ITSM.

## Build

```shell
go build -mod=vendor ./cmd/server
```
