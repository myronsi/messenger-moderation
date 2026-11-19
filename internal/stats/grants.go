package stats

var ReadGrants = []string{
	"GRANT SELECT (created_at) ON users TO moderation_reader;",
	"GRANT SELECT (type, created_at) ON chats TO moderation_reader;",
	"GRANT SELECT (started_at, ended_at) ON calls TO moderation_reader;",
	"GRANT SELECT (state) ON media_jobs TO moderation_reader;",
	"GRANT SELECT (reason, created_at) ON user_reports, sticker_pack_reports, emoji_pack_reports TO moderation_reader;",
	"GRANT SELECT ON message_day_counts TO moderation_reader;",
}
