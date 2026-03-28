-- name: CreateOrganization :one
INSERT INTO organizations (
    name, domain, logo_url, plan, settings
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetOrganizationByDomain :one
SELECT * FROM organizations
WHERE domain = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListOrganizations :many
SELECT * FROM organizations
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateOrganization :one
UPDATE organizations
SET
    name     = COALESCE(sqlc.narg('name'), name),
    domain   = COALESCE(sqlc.narg('domain'), domain),
    logo_url = COALESCE(sqlc.narg('logo_url'), logo_url),
    plan     = COALESCE(sqlc.narg('plan'), plan),
    settings = COALESCE(sqlc.narg('settings'), settings)
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteOrganization :exec
UPDATE organizations
SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
