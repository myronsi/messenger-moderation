# Statistics

The moderation service keeps one row of totals per day in table `daily_stats` of its own database. Rows are
kept forever. They hold numbers only, never who did what, so they contain no personal data.

## How a day is written

- The service checks every 15 minutes whether yesterday (UTC) has a row. If not, it computes the day from the
  Messenger database (read-only user, `MESSENGER_READ_DATABASE_URL`) and stores it. A day that already has a
  row is never changed by this check, so running it again is safe.
- After a pause (the service was down) it fills the missing days, at most the last 31.
- A day computed more than 6 hours after it ended is marked `late`. The numbers that only describe "now" (marked
  "point in time" below) are left empty for such a day, because they would describe a later moment.
- `moderation stats-rerun -from YYYY-MM-DD [-to YYYY-MM-DD]` recomputes days on purpose and overwrites them, for
  example after a fix. Point-in-time numbers then follow the same 6-hour rule.
- Without `MESSENGER_READ_DATABASE_URL` no rows are written and the "today" view answers 503.

## The numbers

Each day runs from 00:00 to 24:00 UTC. "In the day" means the row's time falls in that range. Totals count rows
that exist when the day is computed: an account or chat deleted later is no longer in the total of a recomputed
day.

| Column | Meaning | Source | Week and month |
|---|---|---|---|
| `users_total` | Accounts created before the end of the day | `users.created_at` | last day |
| `users_new` | Accounts created in the day | `users.created_at` | sum |
| `users_active_day` | Accounts with a session used since the start of the day (point in time) | `user_sessions.last_active_at` | average per day |
| `users_active_30d` | Accounts with a session used in the 30 days before the end of the day (point in time) | `user_sessions.last_active_at` | last day |
| `messages` | Messages sent in the day: text, file, voice and sticker, forwards included | `message_day_counts` (messenger-back counts each stored message) | sum |
| `messages_text`, `messages_file`, `messages_voice`, `messages_sticker` | The same by kind; `voice` includes video notes | `message_day_counts` | sum |
| `files_sent` | Files in file messages; an album counts every file | `message_day_counts.attachments` | sum |
| `direct_chats_total`, `groups_total` | Chats of that kind created before the end of the day | `chats` | last day |
| `direct_chats_new`, `groups_new` | Chats of that kind created in the day | `chats.created_at` | sum |
| `calls` | Calls that started in the day (rang and were answered) | `calls.started_at` | sum |
| `call_minutes` | Length of those calls, rounded up to whole minutes; a call still running counts until the end of the day | `calls.started_at`, `ended_at` | sum |
| `files_bytes` | Size of all uploaded files as recorded at upload (point in time) | `attachments.size` | last day |
| `storage_bytes` | Space used in object storage (point in time) | messenger-back internal endpoint (empty until it exists) | last day |
| `messenger_db_bytes`, `moderation_db_bytes` | Size of the two PostgreSQL databases (point in time) | `pg_database_size` | last day |
| `media_jobs_waiting` | Media conversions queued or running (point in time) | `media_jobs.state` | last day |
| `reports_total`, `reports` | Reports against accounts received in the day, total and by reason | `user_reports.created_at`, `reason` | sum |
| `pack_reports_total`, `pack_reports` | Reports against sticker packs received in the day, total and by reason | `sticker_pack_reports` | sum |
| `decisions_total`, `decisions` | Staff decisions in the day, total and by type | empty until decisions exist (milestone M2) | sum |
| `median_decision_seconds` | Median time from case opened to decision, for cases decided in the day | empty until cases exist | average of the daily medians |
| `reversals` | Automatic levels a person reversed in the day | empty until decisions exist | sum |

Messages are not counted from the message store (ScyllaDB): that would be too slow. messenger-back adds every
stored message to a per-day counter instead (`docs/messages.md`, "Day counts", there).

## Who sees what

| | Admins | Moderators |
|---|---|---|
| `GET /api/stats` | all columns | `reports*`, `pack_reports*`, `decisions*`, `median_decision_seconds`, `reversals` |
| `GET /api/stats/today` | today so far and yesterday | the same, moderation columns only |
| CSV download | all columns | moderation columns only |

`GET /api/stats?from=YYYY-MM-DD&to=YYYY-MM-DD&group=day|week|month&format=json|csv`: default the last 30 days
up to yesterday, by day, as JSON; at most 3660 days. Weeks start on Monday and are labelled with that date,
months with their first day. Columns are combined as listed under "Week and month". A period is `late` when one
of its days is.

`GET /api/stats/today` computes today from 00:00 UTC until now, next to yesterday's stored row.

## Retention

| Data | Kept | Job |
|---|---|---|
| `daily_stats` | forever | none |
| `audit_log` | 2 years (a database trigger refuses earlier deletes) | daily, `internal/retention` |
| `incidents` | 2 years after they ended; open incidents stay | daily, `internal/retention` |
| `service_checks` | 30 days, then one uptime figure per component and day in `service_uptime_daily` (kept) | `internal/status` |

Later milestones add their tables to `retention.Rules`: content copies 6 months, cases 2 years.
