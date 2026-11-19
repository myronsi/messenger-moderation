# Messenger moderation

Moderation service and console for Messenger: staff accounts, review of reports, decisions with reasons,
cases, service status and statistics. Plan and decisions: `moderation-console-plan-2026-11.md` (operator's
planning folder), section 0 "Architecture v2".

## Layout

| Path | What |
|---|---|
| `cmd/moderation` | The program: `serve` (default), `migrate`, `create-staff` |
| `internal/config` | Environment configuration, validated on start |
| `internal/db` | PostgreSQL pool and embedded migrations (`internal/db/migrations`) for the `moderation` database |
| `internal/staff` | Staff accounts: Argon2id passwords, required TOTP, 12-hour sessions, roles `admin` and `moderator`, sign-in rate limit |
| `internal/audit` | Append-only audit log (rows can only be deleted after 2 years; enforced by a database trigger) |
| `internal/httpserver` | HTTP API under `/api`, security headers, same-origin check for changes, console files with SPA fallback; `RequireStaff` and `RequireAdmin` for new routes |
| `internal/testdb` | Test helper: a fresh schema per test in the test PostgreSQL |
| `console/` | The console UI; its build output in `console/dist` is embedded into the program |

## Settings

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | The `moderation` database, owned by the service |
| `ENCRYPTION_KEY` | required | 32 random bytes, base64 (`openssl rand -base64 32`); encrypts TOTP secrets |
| `PUBLIC_ORIGIN` | `http://localhost:8090` | The console's origin, for example `https://moderation.messenger.viserix.com`; changes from any other origin are refused |
| `HTTP_ADDR` | `:8090` | Listen address |
| `APP_ENV` | `development` | `development`, `test` or `production` (production needs an https origin) |
| `STAFF_SESSION_TTL` | `12h` | How long a staff session lasts |
| `TRUST_PROXY` | `false` | Use `X-Forwarded-For` for the client address (set behind nginx) |
| `CONSOLE_DIR` | empty | Serve the console from a folder instead of the embedded build (development) |
| `MESSENGER_INTERNAL_URL`, `MESSENGER_INTERNAL_TOKEN` | empty | messenger-back's internal API (`docs/internal-api.md` there) |
| `MESSENGER_READ_DATABASE_URL` | empty | Read-only user on the Messenger database |

## Commands

```sh
go run ./cmd/moderation migrate
go run ./cmd/moderation create-staff -role admin <username>
go run ./cmd/moderation create-staff -role moderator -confidentiality-signed 2026-11-19 <username>
go run ./cmd/moderation
```

`create-staff` reads the password from `STAFF_PASSWORD` or the first line of standard input and prints the
TOTP secret once. Moderators need the date of their signed confidentiality promise.

## Tests

The tests use the test PostgreSQL that `messenger-back` starts with `sh scripts/test-deps.sh`; each test
creates its own schema and drops it afterwards.

```sh
set -a; . ../messenger-back/.env.test; set +a
go test ./...
golangci-lint run ./...
```
