# SecArch Tickets

SecArch Tickets discovers the two supported SecArch request types from CMDB,
groups them by the owning super department, and stores a durable PostgreSQL
snapshot. A full Open + Closed synchronization runs at service startup. Opening
or reloading the page first shows the stored snapshot, then requests an Open-ticket
synchronization through `POST /api/tickets/refresh` when the shared waiting period
has elapsed. Concurrent requests share one synchronization per service instance;
success cooldown, failure backoff, and circuit breaker controls also apply to page
reloads. The page shows synchronization status and any remaining wait, without a
separate Refresh button. There is no periodic synchronization or automatic retry
when the countdown ends; reload the page to try again. Existing data remains
available if synchronization fails, and `GET /api/tickets` only reads the snapshot.

## Routes

- `GET /`
- `GET /api/tickets`
- `POST /api/tickets/refresh`
- `PUT /api/tickets/:ticket_number/expected-date`
- `GET /api/tickets/:ticket_number/updates`
- `POST /api/tickets/:ticket_number/updates`
- `GET /api/statistics/closed?start=YYYY-MM-DD&end=YYYY-MM-DD`
- `GET /healthz`

With OIDC enabled, all routes except `/healthz`, `/static/*`, and the OIDC
login/callback/logout endpoints require an authenticated OIDC session. Users with
the configured admin App Role can access every ticket. Other users can access only
rows whose `reporter` matches their configured OIDC username claim; Department does
not affect access. This same row-level check is enforced for lists, statistics,
expected-date changes, and progress-note reads/writes.

Setting `oidc.enabled` to `false` in `app.json` allows all visitors to access every ticket and perform
all ticket operations without logging in. It skips OIDC configuration validation
and provider discovery, and removes the login routes and account controls from
the page. OIDC is enabled by default; invalid switch values stop startup.

## Configuration

All application settings come from one JSON file. Select it with `-config`:

```shell
./server -config /d/d1/secarch-tickets/config/app.json
```

The default path is `/d/d1/secarch-tickets/config/app.json`. Complete configurations
for QA and production are in `config/qa/app.json` and `config/prod/app.json`; deploy
one of them as the selected file. The service does not load the former `db.json`,
`cmdb.json`, or `proxy` files. `APP_*`, `OIDC_*`, and HTTP proxy environment variables
do not override application settings. Changing the JSON file requires a restart.

| Section | Settings |
| --- | --- |
| `server` | `environment` (`qa` or `prod`, default `qa`) and `listen_addr` (default `:8443`). |
| `tls` | `server_cert`, `server_key`, `client_cert`, `client_key`, and `ca_cert` paths. Relative paths are resolved against the directory containing `app.json`. |
| `db` | Database servers, port, database/user names, TLS mode, connection limits, and timeouts. |
| `cmdb` | Ticket/objects API URLs, ticket browse URL, batching limits, HTTP timeout, cooldown, backoff, and circuit breaker settings. |
| `oidc` | Authentication switch, provider/application settings, claims, credentials, and session settings. |
| `proxy` | `http_proxy`, `https_proxy`, `no_proxy`, and `cmdb_enabled`. |

The existing database and CMDB field names are preserved inside their sections.
The `tls` section supplies the shared client certificate, key, and CA to both
clients. Database connections use the `db_servers` array in order, accepting only
a writable PostgreSQL session so new pool connections can follow a failover.

OIDC is explicitly disabled in both supplied configurations while IAM setup is
pending. The minimal disabled section is:

```json
"oidc": {
  "enabled": false
}
```

With `enabled: false`, all remaining OIDC settings are ignored and no OIDC service
is contacted. Database, CMDB, and TLS settings are still required. Omitting
`enabled` defaults to `true`, which requires valid OIDC settings. Invalid JSON,
unknown application fields, and invalid enabled OIDC settings stop startup.

Once IAM is ready, edit the `oidc` section in the same file:

```json
"oidc": {
  "enabled": true,
  "issuer_url": "https://login.partner.microsoftonline.cn/<tenant-id>/v2.0",
  "client_id": "<IAM-provided-client-id>",
  "client_secret": "<IAM-provided-client-secret-value>",
  "redirect_url": "https://secarch-tickets.srv.ms.com.cn/auth/callback",
  "username_claim": "samaccountname",
  "groups_claim": "roles",
  "admin_group": "SecArchTicketsAdmin",
  "scopes": ["openid", "profile", "email"],
  "session_ttl": "8h",
  "session_cookie": "secarch_oidc_session",
  "session_secret": "<locally-generated-random-secret-of-at-least-32-bytes>",
  "http_timeout_seconds": 30
}
```

`issuer_url`, `client_id`, `client_secret`, `redirect_url`, and `session_secret` are
required when enabled. The other fields default to the values above. The client
secret is the credential value supplied by IAM, not its ID. The session secret is
separate, must contain at least 32 bytes, and must be shared by application replicas.
Changing it invalidates existing login flows and sessions. Keep deployed credentials
in the deployment configuration; do not copy them into examples or logs.

The provider must return the selected username claim in the ID token. With the
shown settings, the ID token must contain `"roles": ["SecArchTicketsAdmin"]` for
full ticket access. Other users see only tickets matching their `samaccountname`.
The `groups_claim` and `admin_group` setting names are retained for compatibility
with the application's existing claim mapping; the defaults refer to Entra App Roles.
The login flow uses state, nonce, and PKCE; session cookies are encrypted,
authenticated, `Secure`, and `HttpOnly`.

OIDC discovery, token exchange, and signing-key requests all use the proxy values
from this file. Empty proxy URLs mean direct connections, with no environment
fallback. `no_proxy` follows Go's normal hostname, IP, CIDR, and port matching rules;
localhost and loopback requests use direct connections. CMDB continues to connect
directly by default (`proxy.cmdb_enabled: false`); setting it to `true` applies the
same proxy policy to CMDB. Database connections do not use HTTP proxies. The service
does not modify process or machine proxy environment variables.

For systemd, the application configuration entry is just the path on `ExecStart`:

```ini
[Service]
ExecStart=/d/d1/secarch-tickets/server -config /d/d1/secarch-tickets/config/app.json
```

When deploying this version, remove the old application/proxy `Environment=` and
`EnvironmentFile=` entries from the unit once their values have been moved to
`app.json`. The unit continues to manage the process user, restart policy, and
other systemd controls. There is no separate environment file to maintain.

The service stores tickets in `secarch_tickets.tickets` within the existing
`ai_info_db01` PostgreSQL database. CMDB fields are refreshed in place and are
never hard-deleted by this service. Expected dates and local progress notes are
maintained locally and are never written back to ITSM.

## Build

```shell
go build -mod=vendor ./cmd/server
```
