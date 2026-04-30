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
SET first_name = COALESCE(sqlc.narg('first_name'), first_name),
    last_name = COALESCE(sqlc.narg('last_name'), last_name),
    personal_email = COALESCE(sqlc.narg('personal_email'), personal_email),
    school_email = COALESCE(sqlc.narg('school_email'), school_email),
    phone = COALESCE(sqlc.narg('phone'), phone),
    grad_year = COALESCE(sqlc.narg('grad_year'), grad_year),
    role = COALESCE(sqlc.narg('role'), role)
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
SET name = COALESCE(sqlc.narg('name'), name)
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
SELECT * FROM events_with_org_ids WHERE eid = $1;

-- name: ListEvents :many
SELECT * FROM events_with_org_ids ORDER BY event_time DESC LIMIT $1 OFFSET $2;

-- name: ListEventsByOrg :many
SELECT e.*
FROM events_with_org_ids e
JOIN event_hosting eh ON e.eid = eh.eid
WHERE eh.oid = $1
ORDER BY e.event_time DESC
LIMIT $2 OFFSET $3;

-- name: CreateEvent :one
INSERT INTO events (title, location, event_time, description)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateEvent :one
UPDATE events
SET title = COALESCE(sqlc.narg('title'), title),
    location = COALESCE(sqlc.narg('location'), location),
    event_time = COALESCE(sqlc.narg('event_time'), event_time),
    description = COALESCE(sqlc.narg('description'), description)
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
SELECT EXISTS (
    SELECT 1
    FROM event_registrations er
    WHERE er.uid = $1
      AND er.eid = $2
      AND er.is_admin = TRUE
)
OR EXISTS (
    SELECT 1
    FROM event_hosting eh
    JOIN org_members om ON om.oid = eh.oid
    WHERE eh.eid = $2
      AND om.uid = $1
      AND om.is_admin = TRUE
);

-- name: GetUserEvents :many
SELECT e.*, er.is_attending, er.is_admin, er.date_registered
FROM events_with_org_ids e
JOIN event_registrations er ON e.eid = er.eid
WHERE er.uid = $1
ORDER BY e.event_time DESC;

-- Bot Token Queries

-- name: GetBotTokenByID :one
SELECT * FROM bot_tokens WHERE token_id = $1;

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

-- Link Queries

-- name: CreateLink :one
INSERT INTO links (endpoint_url, dest_url, oid)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetLinkByLID :one
SELECT * FROM links WHERE lid = $1;

-- name: GetLinkByEndpointURL :one
SELECT * FROM links WHERE endpoint_url = $1;

-- name: UpdateLink :one
UPDATE links
SET endpoint_url = COALESCE(sqlc.narg('endpoint_url'), endpoint_url),
    dest_url = COALESCE(sqlc.narg('dest_url'), dest_url)
WHERE lid = $1
RETURNING *;

-- name: DeleteLink :exec
DELETE FROM links WHERE lid = $1;

-- name: ListLinksByOrg :many
SELECT * FROM links WHERE oid = $1 ORDER BY created_at DESC;

-- name: LogLinkVisit :one
INSERT INTO link_visits (lid, uid)
VALUES ($1, $2)
RETURNING *;

-- name: GetTotalVisits :one
SELECT COUNT(*) FROM link_visits WHERE lid = $1;
