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
| `internal/appdb` | Read-only connection to the Messenger database (`appdbtest` holds the copy of the Messenger tables the tests use) |
| `internal/review` | Review queue, account page and account search; every call writes a data-access row to the audit log |
| `internal/httpserver` | HTTP API under `/api`, security headers, same-origin check for changes, console files with SPA fallback; `RequireStaff` and `RequireAdmin` for new routes |
| `internal/messenger` | Client for messenger-back's internal API (`/internal/v1`): set status, dismiss reports, hide or unhide packs, delete accounts, pack images |
| `internal/decisions` | Staff actions with a guideline rule and a text for the user, each audited with before and after; cases list and confirm or reverse; pack pages |
| `internal/cases` | Cases with a 72-hour deadline, opened by a watcher every minute for automatic restricted or suspended levels and packs hidden after reports (last 30 days) |
| `internal/status` | Status checks every minute, incidents after 3 failures, monitoring gaps, email alerts, `/status.json` (public) and `/api/status` (admins), daily uptime |
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
| `MESSENGER_READ_DATABASE_URL` | empty | Read-only user on the Messenger database; without it the queue, account pages and search answer `503` |
| `ENVIRONMENT_NAME` | `development` | `development`, `staging` or `production`; shown as a coloured badge in the console |
| `MESSENGER_API_READY_URL` | empty | The API readiness address, for example `http://api:8080/readyz`; checks the API and every store behind it |
| `MESSENGER_WORKER_HEALTH_URL` | empty | The worker health address, for example `http://worker:8081/healthz` |
| `LIVEKIT_HEALTH_URL` | empty | LiveKit, when calls are on |
| `CHECK_INTERVAL` | `1m` | How often the status checks run |
| `STATUS_CORS_ORIGINS` | empty | Sites allowed to read `/status.json`, for example the help site |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` | port 587 | Mail server for alerts |
| `ALERT_EMAIL` | empty | Where incident alerts go; needs `SMTP_HOST` and `SMTP_FROM` |

## Messenger database access

The service opens the Messenger database with `default_transaction_read_only=on`. The queue, account page and
search read only these columns, so this is the smallest grant for `moderation_reader` (see `docs/deploy.md`);
column grants keep `password_hash` and other private fields out:

```sql
GRANT SELECT (id, username, display_name, bio, created_at, last_seen_at) ON users TO moderation_reader;
GRANT SELECT ON user_reports, user_moderation, user_moderation_events TO moderation_reader;
```

## API

All routes need a staff session (cookie `mod_session`); moderators and admins may use them.

| Route | What | Audit action |
|---|---|---|
| `GET /api/queue` | Accounts with reports newer than the last staff review, or a restricted or suspended status set after it; most urgent first (status level, newest report that counts, score). Query: `status` and `reason` (comma-separated), `opened_after`, `opened_before` (RFC 3339, when the account entered the queue), `limit` (1 to 100, default 50), `cursor` | `queue.viewed` with the listed user IDs |
| `GET /api/users/{id}` | Profile, current status with probation and appeal block, status history, all reports with reporter, reason, note, context, weight and profile snapshot | `user.viewed`, target `user` |
| `GET /api/users?q=` | Up to 20 accounts by username prefix, display name or ID (at least 2 characters or an ID; a leading `@` is ignored) | `users.searched` with the query and the user IDs |

Staff decisions arrive with M2; until then no account counts as reviewed.

## Commands

```sh
go run ./cmd/moderation migrate
go run ./cmd/moderation create-staff -role admin <username>
go run ./cmd/moderation create-staff -role moderator -confidentiality-signed 2026-11-19 <username>
go run ./cmd/moderation
```

`create-staff` reads the password from `STAFF_PASSWORD` or the first line of standard input and prints the
TOTP secret once. Moderators need the date of their signed confidentiality promise.

## Console

```sh
cd console
npm install --prefer-offline
npm run dev
```

The dev server proxies `/api` to `http://localhost:8090`. `npm run build` writes `console/dist`, which the Go program embeds. Checks: `npm run typecheck`, `npm run lint`, `npm test`.

## Tests

The tests use the test PostgreSQL that `messenger-back` starts with `sh scripts/test-deps.sh`; each test
creates its own schema and drops it afterwards.

```sh
set -a; . ../messenger-back/.env.test; set +a
go test ./...
golangci-lint run ./...
```
