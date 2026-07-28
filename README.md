# SecArch Tickets

SecArch Tickets centralizes SecArch case tracking and expected-date updates.
Ticket details are refreshed from CMDB and stored in PostgreSQL.

## Routes

- `GET /`
- `GET /api/tickets`
- `POST /api/tickets`
- `PUT /api/tickets/:ticket_number`
- `DELETE /api/tickets/:ticket_number`
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

The service stores tickets in `secarch_tickets.tickets` within the existing
`ai_info_db01` PostgreSQL database.

## Build

```shell
go build -mod=vendor ./cmd/server
```
