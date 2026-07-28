# SecArch Tickets

SecArch Tickets centralizes SecArch case tracking and expected-date updates.
Ticket details are refreshed from CMDB and stored in PostgreSQL.

## Routes

- `GET /secarch/tickets`
- `GET /api/secarch/tickets`
- `POST /api/secarch/tickets`
- `PUT /api/secarch/tickets/:ticket_number`
- `DELETE /api/secarch/tickets/:ticket_number`
- `GET /healthz`

## Configuration

The service uses these environment variables:

- `APP_ENV`: `qa` or `prod`; defaults to `qa`.
- `APP_CONFIG_DIR`: directory containing `db.json` and `cmdb.json`.
- `APP_TLS_DIR`: directory containing server and client certificates.
- `APP_LISTEN_ADDR`: HTTPS listen address; defaults to `:8443`.

The service stores tickets in `secarch_tickets.tickets` within the existing
`ai_info_db01` PostgreSQL database.

## Build

```shell
go build -mod=vendor ./cmd/server
```
