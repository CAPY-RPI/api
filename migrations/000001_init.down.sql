DROP TRIGGER IF EXISTS update_events_modtime ON events;
DROP TRIGGER IF EXISTS update_orgs_modtime ON organizations;
DROP TRIGGER IF EXISTS update_users_modtime ON users;

DROP INDEX IF EXISTS idx_bot_tokens_active;

DROP TABLE IF EXISTS bot_tokens;
DROP TABLE IF EXISTS event_registrations;
DROP TABLE IF EXISTS event_hosting;
DROP TABLE IF EXISTS org_members;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS update_modified_column();
DROP TYPE IF EXISTS user_role;
