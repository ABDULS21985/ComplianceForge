-- name: CreateUser :one
INSERT INTO users (
    organization_id, email, password_hash, first_name, last_name, role, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListUsersByOrganization :many
SELECT * FROM users
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateUser :one
UPDATE users
SET
    email         = COALESCE(sqlc.narg('email'), email),
    first_name    = COALESCE(sqlc.narg('first_name'), first_name),
    last_name     = COALESCE(sqlc.narg('last_name'), last_name),
    role          = COALESCE(sqlc.narg('role'), role),
    status        = COALESCE(sqlc.narg('status'), status),
    password_hash = COALESCE(sqlc.narg('password_hash'), password_hash)
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteUser :exec
UPDATE users
SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateLastLogin :exec
UPDATE users
SET last_login_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
