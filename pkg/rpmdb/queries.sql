-- name: CountByName :one
SELECT COUNT(*) FROM Name WHERE name = ?;

-- name: ListInstalledNames :many
SELECT DISTINCT name FROM Name ORDER BY name;

-- name: ListInstalledProvides :many
SELECT DISTINCT key FROM Providename ORDER BY key;
