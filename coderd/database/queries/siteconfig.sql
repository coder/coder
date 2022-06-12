-- name: InsertDeploymentID :exec
INSERT INTO site_config (key, value) VALUES ('deployment_id', $1);

-- name: GetDeploymentID :one
SELECT value FROM site_config WHERE key = 'deployment_id';
