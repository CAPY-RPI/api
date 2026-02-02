-- name: GetUserByID :one
SELECT * FROM users WHERE uid = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE personal_email = $1 OR school_email = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY last_name, first_name LIMIT $1 OFFSET $2;

-- name: CreateUser :one
INSERT INTO users (first_name, last_name, personal_email, school_email, phone, grad_year, role)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET first_name = COALESCE($2, first_name),
    last_name = COALESCE($3, last_name),
    personal_email = COALESCE($4, personal_email),
    school_email = COALESCE($5, school_email),
    phone = COALESCE($6, phone),
    grad_year = COALESCE($7, grad_year),
    role = COALESCE($8, role)
WHERE uid = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE uid = $1;

-- name: GetOrganizationByID :one
SELECT * FROM organizations WHERE oid = $1;

-- name: ListOrganizations :many
SELECT * FROM organizations ORDER BY name LIMIT $1 OFFSET $2;

-- name: CreateOrganization :one
INSERT INTO organizations (name)
VALUES ($1)
RETURNING *;

-- name: UpdateOrganization :one
UPDATE organizations
SET name = COALESCE($2, name)
WHERE oid = $1
RETURNING *;

-- name: DeleteOrganization :exec
DELETE FROM organizations WHERE oid = $1;

-- name: GetOrgMembers :many
SELECT u.*, om.is_admin, om.date_joined, om.last_active
FROM users u
JOIN org_members om ON u.uid = om.uid
WHERE om.oid = $1
ORDER BY u.last_name, u.first_name;

-- name: AddOrgMember :exec
INSERT INTO org_members (uid, oid, is_admin)
VALUES ($1, $2, $3)
ON CONFLICT (uid, oid) DO UPDATE SET is_admin = $3;

-- name: RemoveOrgMember :exec
DELETE FROM org_members WHERE uid = $1 AND oid = $2;

-- name: IsOrgAdmin :one
SELECT is_admin FROM org_members WHERE uid = $1 AND oid = $2;

-- name: GetUserOrganizations :many
SELECT o.*, om.is_admin, om.date_joined
FROM organizations o
JOIN org_members om ON o.oid = om.oid
WHERE om.uid = $1
ORDER BY o.name;

-- name: GetEventByID :one
SELECT * FROM events WHERE eid = $1;

-- name: ListEvents :many
SELECT * FROM events ORDER BY event_time DESC LIMIT $1 OFFSET $2;

-- name: ListEventsByOrg :many
SELECT e.*
FROM events e
JOIN event_hosting eh ON e.eid = eh.eid
WHERE eh.oid = $1
ORDER BY e.event_time DESC
LIMIT $2 OFFSET $3;

-- name: CreateEvent :one
INSERT INTO events (location, event_time, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateEvent :one
UPDATE events
SET location = COALESCE($2, location),
    event_time = COALESCE($3, event_time),
    description = COALESCE($4, description)
WHERE eid = $1
RETURNING *;

-- name: DeleteEvent :exec
DELETE FROM events WHERE eid = $1;

-- name: AddEventHost :exec
INSERT INTO event_hosting (eid, oid)
VALUES ($1, $2)
ON CONFLICT (eid, oid) DO NOTHING;

-- name: GetEventRegistrations :many
SELECT u.*, er.is_attending, er.is_admin, er.date_registered
FROM users u
JOIN event_registrations er ON u.uid = er.uid
WHERE er.eid = $1
ORDER BY er.date_registered;

-- name: RegisterForEvent :exec
INSERT INTO event_registrations (uid, eid, is_attending)
VALUES ($1, $2, $3)
ON CONFLICT (uid, eid) DO UPDATE SET is_attending = $3;

-- name: UnregisterFromEvent :exec
DELETE FROM event_registrations WHERE uid = $1 AND eid = $2;

-- name: IsEventAdmin :one
SELECT is_admin FROM event_registrations WHERE uid = $1 AND eid = $2;

-- name: GetUserEvents :many
SELECT e.*, er.is_attending, er.is_admin, er.date_registered
FROM events e
JOIN event_registrations er ON e.eid = er.eid
WHERE er.uid = $1
ORDER BY e.event_time DESC;

-- Bot Token Queries

-- name: GetBotTokenByHash :one
SELECT * FROM bot_tokens WHERE token_hash = $1 AND is_active = true;

-- name: CreateBotToken :one
INSERT INTO bot_tokens (token_hash, name, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListBotTokens :many
SELECT token_id, name, created_by, created_at, last_used_at, expires_at, is_active
FROM bot_tokens
ORDER BY created_at DESC;

-- name: RevokeBotToken :exec
UPDATE bot_tokens SET is_active = false WHERE token_id = $1;

-- name: UpdateBotTokenLastUsed :exec
UPDATE bot_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE token_id = $1;
