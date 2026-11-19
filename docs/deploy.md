# Deploying the moderation service

`main` deploys to **staging**; a tag `vX.Y.Z` on a commit in `main` deploys to **production**
(`.github/workflows/deploy.yml`). The workflow builds one Docker image (server and console), pushes it to
`ghcr.io/myronsi/messenger-moderation`, uploads `deploy/` to the server and runs `deploy/deploy.sh`, which pulls
the image, runs the migrations and starts the container. Run the workflow by hand to roll back to an earlier
tag.

## One-time setup per environment (operator)

Do this once on the staging host and once on the production host.

### 1. Databases

As the PostgreSQL owner of the Messenger stack:

```sql
CREATE ROLE moderation LOGIN PASSWORD '<random>';
CREATE DATABASE moderation OWNER moderation;

CREATE ROLE moderation_reader LOGIN PASSWORD '<random>';
GRANT CONNECT ON DATABASE messenger TO moderation_reader;
\c messenger
GRANT USAGE ON SCHEMA public TO moderation_reader;
```

The reader gets **only the tables and columns a feature needs**, never `ALL TABLES` and no default
privileges: password hashes, sessions, recovery data and messages stay out of reach. The moderation service
never writes to the Messenger database.

### 1a. Read grants

Run the grants of every feature that is deployed. Each pull request that reads another table or column adds
its grant here, in the same change.

| Feature | Grants |
|---|---|
| Review queue, account page, search | `GRANT SELECT (id, username, display_name, bio, created_at, last_seen_at) ON users TO moderation_reader;`<br>`GRANT SELECT ON user_reports, user_moderation, user_moderation_events TO moderation_reader;` |
| Decisions, cases and pack pages (M2) | `GRANT SELECT (id, owner_id, builtin_key, title, slug, hidden_at, hidden_by, deleted_at, created_at) ON sticker_packs TO moderation_reader;`<br>`GRANT SELECT (id, pack_id, attachment_id, emoji, position, is_animated, deleted_at) ON stickers TO moderation_reader;`<br>`GRANT SELECT (pack_id, reporter_id, reason, created_at) ON sticker_pack_reports TO moderation_reader;`<br>`GRANT SELECT (id, owner_id, chat_id, title, slug, hidden_at, hidden_by, deleted_at, created_at) ON emoji_packs TO moderation_reader;`<br>`GRANT SELECT (id, pack_id, attachment_id, shortcode, position, is_animated, deleted_at) ON custom_emoji TO moderation_reader;`<br>`GRANT SELECT (pack_id, reporter_id, reason, created_at) ON emoji_pack_reports TO moderation_reader;`<br>The decision columns of `user_moderation` and `user_moderation_events` and `user_reports.dismissed_at` are covered by the table grants above. |
| Content reports (M3) | `GRANT SELECT (id, reporter_id, kind, chat_id, message_id, target_id, reason, note, content, created_at, withdrawn_at) ON content_reports TO moderation_reader;`<br>Reporter and sender names come from the `users` columns above. The reader gets no message tables: content is visible only through these copies, which Messenger deletes after 6 months. |
| Review requests (M4) | `GRANT SELECT (id, user_id, status, text, created_at, closed_at, close_reason) ON account_review_requests TO moderation_reader;`<br>Notices by email live only in the `moderation` database and need no grant. |
| Daily statistics ([statistics.md](statistics.md)): counts only, no names, no session tokens or addresses | `GRANT SELECT (created_at) ON users TO moderation_reader;`<br>`GRANT SELECT (user_id, last_active_at) ON user_sessions TO moderation_reader;`<br>`GRANT SELECT (type, created_at) ON chats TO moderation_reader;`<br>`GRANT SELECT (started_at, ended_at) ON calls TO moderation_reader;`<br>`GRANT SELECT (size) ON attachments TO moderation_reader;`<br>`GRANT SELECT (state) ON media_jobs TO moderation_reader;`<br>`GRANT SELECT (reason, created_at) ON user_reports, sticker_pack_reports TO moderation_reader;`<br>`GRANT SELECT ON message_day_counts TO moderation_reader;` (after messenger-back's migration `message_day_counts`) |

Check what the reader can see:

```sql
SELECT table_name, column_name FROM information_schema.column_privileges
WHERE grantee = 'moderation_reader' ORDER BY 1, 2;
```

### 2. Messenger internal API

In the Messenger stack's `.env` set `INTERNAL_ADDR=:8082` and `INTERNAL_TOKEN=<openssl rand -base64 48>`, then
redeploy Messenger. The port is not published; only containers on the stack's network reach it.

### 3. Server folder and settings

Staging and production run on the same host, next to the two Messenger stacks. Each environment has its own
folder, and the folder name is its compose project:

| Environment | Folder | `MODERATION_PORT` | `MESSENGER_NETWORK` |
|---|---|---|---|
| production | `/opt/messenger-moderation` | `8190` | `messenger-go_default` (stack in `/opt/messenger-go`) |
| staging | `/opt/messenger-moderation-staging` | `8191` | `<COMPOSE_PROJECT_NAME of /opt/messenger-staging>_default` |

```sh
sudo mkdir -p /opt/messenger-moderation /opt/messenger-moderation-staging
sudo chown <deploy user> /opt/messenger-moderation /opt/messenger-moderation-staging
docker network ls
```

Create `.env` in each folder (mode 600) from `deploy/env.example`: port, network, the two database URLs,
`ENCRYPTION_KEY` (`openssl rand -base64 32`), the internal token, the environment name and origin, SMTP and
`ALERT_EMAIL`. Compose reads `MODERATION_PORT` and `MESSENGER_NETWORK` from that file too.

### 4. nginx

nginx terminates TLS behind the PROXY protocol listener of the static sites, so the visitor's address is
`$proxy_protocol_addr`. Replace the two static moderation server blocks with:

```nginx
server {
    listen 127.0.0.1:8443 ssl proxy_protocol;
    http2 on;
    server_name moderation.staging.messenger.viserix.com;
    ssl_certificate /etc/letsencrypt/live/messenger-static-sites/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/messenger-static-sites/privkey.pem;
    client_max_body_size 1m;
    location / {
        proxy_pass http://127.0.0.1:8191;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_protocol_addr;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

and the same for `moderation.messenger.viserix.com` with port `8190`.

### 5. GitHub environments

In the repository settings, environments `staging` and `production`, add:

| Secret | Value |
|---|---|
| `DEPLOY_SSH_HOST` | Host name of the server |
| `DEPLOY_SSH_USER` | A user that may run `docker` and write `/opt/messenger-moderation` |
| `DEPLOY_SSH_KEY` | Private key of that user |
| `DEPLOY_SSH_KNOWN_HOSTS` | `ssh-keyscan <host>` output |

Environment **variable** `DEPLOY_PATH`: set it to `/opt/messenger-moderation-staging` in `staging`; production
uses the default `/opt/messenger-moderation`. The old `SSH_KEY` secret of the static page is no longer used and
can be removed.

### 6. First admin

In the environment's folder (`/opt/messenger-moderation-staging` or `/opt/messenger-moderation`):

```sh
docker compose exec -it moderation /app/moderation create-staff -role admin <username>
```

This works after the first deploy, which records the running image in `.env` (`MODERATION_IMAGE`). Before that, use the container directly:

```sh
sudo docker exec -it <folder name>-moderation-1 /app/moderation create-staff -role admin <username>
```

It asks for the password and prints the authenticator secret once; add it to an authenticator app right away.

### 7. Outside check

`.github/workflows/uptime.yml` checks every 5 minutes that the app and `/status.json` are reachable from the
internet. GitHub emails the repository owner when a scheduled run fails. Override the addresses with the
repository variables `UPTIME_APP_URL` and `UPTIME_STATUS_URL`.

## Help site status banner

Add the help site's origin to `STATUS_CORS_ORIGINS` so it can read `/status.json`.
