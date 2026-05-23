-- Read queries
-- name: CountByName :one
SELECT COUNT(*) FROM Name WHERE name = ?;

-- name: ListInstalledNames :many
SELECT DISTINCT name FROM Name ORDER BY name;

-- name: ListInstalledProvides :many
SELECT DISTINCT key FROM Providename ORDER BY key;

-- name: CountPackages :one
SELECT COUNT(*) FROM Packages;

-- Write queries
-- name: InsertPackage :exec
INSERT INTO Packages (hnum, blob) VALUES (?, ?);

-- name: InsertName :exec
INSERT INTO Name (name, hnum) VALUES (?, ?);

-- name: InsertProvidename :exec
INSERT INTO Providename (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertRequirename :exec
INSERT INTO Requirename (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertConflictname :exec
INSERT INTO Conflictname (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertObsoletename :exec
INSERT INTO Obsoletename (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertBasenames :exec
INSERT INTO Basenames (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertDirnames :exec
INSERT INTO Dirnames (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertFiledigests :exec
INSERT INTO Filedigests (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertTriggername :exec
INSERT INTO Triggername (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertSha1header :exec
INSERT INTO Sha1header (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertInstalltid :exec
INSERT INTO Installtid (key, hnum, idx) VALUES (?, ?, ?);

-- name: InsertSigmd5 :exec
INSERT INTO Sigmd5 (key, hnum, idx) VALUES (?, ?, ?);

-- name: GetMaxHnum :one
SELECT COALESCE(MAX(hnum), 0) FROM Packages;
