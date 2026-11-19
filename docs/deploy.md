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

Check what the reader can see:

```sql
SELECT table_name, column_name FROM information_schema.column_privileges
WHERE grantee = 'moderation_reader' ORDER BY 1, 2;
```

### 2. Messenger internal API

In the Messenger stack's `.env` set `INTERNAL_ADDR=:8082` and `INTERNAL_TOKEN=<openssl rand -base64 48>`, then
redeploy Messenger. The port is not published; only containers on the stack's network reach it.

### 3. Server folder and settings

```sh
sudo mkdir -p /opt/messenger-moderation && sudo chown <deploy user> /opt/messenger-moderation
```

After the first workflow run uploaded `env.example`, copy it to `/opt/messenger-moderation/.env` (mode 600) and
fill it in: the two database URLs, `ENCRYPTION_KEY` (`openssl rand -base64 32`), the internal token, the
environment name and origin, SMTP and `ALERT_EMAIL`. `MESSENGER_NETWORK` in the environment of `deploy.sh`
defaults to `messenger-go_default`, the network of the Messenger compose project; check it with
`docker network ls`.

### 4. nginx

Route the console's host name to the container, which listens on `127.0.0.1:8090`:

```nginx
server {
    server_name moderation.messenger.viserix.com;
    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

(staging: `moderation.staging.messenger.viserix.com`). TLS as for the other sites.

### 5. GitHub environments

In the repository settings, environments `staging` and `production`, add:

| Secret | Value |
|---|---|
| `DEPLOY_SSH_HOST` | Host name of the server |
| `DEPLOY_SSH_USER` | A user that may run `docker` and write `/opt/messenger-moderation` |
| `DEPLOY_SSH_KEY` | Private key of that user |
| `DEPLOY_SSH_KNOWN_HOSTS` | `ssh-keyscan <host>` output |

Optional variable `DEPLOY_PATH` if the folder is not `/opt/messenger-moderation`. The old `SSH_KEY` secret of
the static page is no longer used and can be removed.

### 6. First admin

```sh
cd /opt/messenger-moderation
MODERATION_IMAGE=<image> docker compose run --rm moderation create-staff -role admin <username>
```

It asks for the password and prints the authenticator secret once; add it to an authenticator app right away.

### 7. Outside check

`.github/workflows/uptime.yml` checks every 5 minutes that the app and `/status.json` are reachable from the
internet. GitHub emails the repository owner when a scheduled run fails. Override the addresses with the
repository variables `UPTIME_APP_URL` and `UPTIME_STATUS_URL`.

## Help site status banner

Add the help site's origin to `STATUS_CORS_ORIGINS` so it can read `/status.json`.
