-- schema.sql
-- Database Schema for CAPY (Club Assistant in Python)

-- 1. ENUMs & Functions
DO $$ BEGIN
    CREATE TYPE user_role AS ENUM ('student', 'alumni', 'faculty', 'external');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.date_modified = CURRENT_DATE;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 2. Tables
CREATE TABLE IF NOT EXISTS users (
    uid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    personal_email TEXT UNIQUE,
    school_email TEXT UNIQUE,
    phone TEXT,
    grad_year INT,
    role user_role DEFAULT 'student',
    date_created DATE DEFAULT CURRENT_DATE,
    date_modified DATE DEFAULT CURRENT_DATE
);

CREATE TABLE IF NOT EXISTS organizations (
    oid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    date_created DATE DEFAULT CURRENT_DATE,
    date_modified DATE DEFAULT CURRENT_DATE
);

CREATE TABLE IF NOT EXISTS org_members (
    uid UUID REFERENCES users(uid) ON DELETE CASCADE,
    oid UUID REFERENCES organizations(oid) ON DELETE CASCADE,
    is_admin BOOLEAN DEFAULT FALSE, 
    date_joined DATE DEFAULT CURRENT_DATE,
    last_active DATE DEFAULT CURRENT_DATE,
    PRIMARY KEY (uid, oid)
);

CREATE TABLE IF NOT EXISTS events (
    eid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location TEXT,
    event_time TIMESTAMP,
    description TEXT,
    date_created DATE DEFAULT CURRENT_DATE,
    date_modified DATE DEFAULT CURRENT_DATE
);

CREATE TABLE IF NOT EXISTS event_hosting (
    eid UUID REFERENCES events(eid) ON DELETE CASCADE,
    oid UUID REFERENCES organizations(oid) ON DELETE CASCADE,
    PRIMARY KEY (eid, oid)
);

CREATE TABLE IF NOT EXISTS event_registrations (
    uid UUID REFERENCES users(uid) ON DELETE CASCADE,
    eid UUID REFERENCES events(eid) ON DELETE CASCADE,
    is_attending BOOLEAN DEFAULT FALSE,
    is_admin BOOLEAN DEFAULT FALSE,
    date_registered DATE DEFAULT CURRENT_DATE,
    PRIMARY KEY (uid, eid)
);

-- 3. Bot Tokens (global access for M2M authentication)
CREATE TABLE IF NOT EXISTS bot_tokens (
    token_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash TEXT NOT NULL,              -- bcrypt hash of the token
    name TEXT NOT NULL,                    -- human-readable name for the bot
    created_by UUID NOT NULL REFERENCES users(uid),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,                  -- NULL = never expires
    is_active BOOLEAN DEFAULT TRUE
);

CREATE INDEX IF NOT EXISTS idx_bot_tokens_active ON bot_tokens(is_active) WHERE is_active = TRUE;

-- 4. Triggers
DROP TRIGGER IF EXISTS update_users_modtime ON users;
CREATE TRIGGER update_users_modtime BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_modified_column();

DROP TRIGGER IF EXISTS update_orgs_modtime ON organizations;
CREATE TRIGGER update_orgs_modtime BEFORE UPDATE ON organizations FOR EACH ROW EXECUTE FUNCTION update_modified_column();

DROP TRIGGER IF EXISTS update_events_modtime ON events;
CREATE TRIGGER update_events_modtime BEFORE UPDATE ON events FOR EACH ROW EXECUTE FUNCTION update_modified_column();