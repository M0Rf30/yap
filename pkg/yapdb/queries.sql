-- Insert a new package record.
-- name: InsertPackage :one
INSERT INTO packages (name, epoch, version, release, arch, format, install_time, summary)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- Insert a file record for a package.
-- name: InsertFile :exec
INSERT INTO files (package_id, path, mode, is_dir, is_symlink, link_target, sha256)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- Insert a capability record.
-- name: InsertCap :exec
INSERT INTO caps (package_id, kind, name, flags, version)
VALUES (?, ?, ?, ?, ?);

-- Delete a package by name and arch (cascades to files and caps).
-- name: DeletePackageByNameArch :exec
DELETE FROM packages WHERE name = ? AND arch = ?;

-- List all installed packages (without files/caps).
-- name: ListPackages :many
SELECT id, name, epoch, version, release, arch, format, install_time, summary
FROM packages
ORDER BY name ASC;

-- Look up a package by name and arch.
-- name: LookupPackageByNameArch :one
SELECT id, name, epoch, version, release, arch, format, install_time, summary
FROM packages
WHERE name = ? AND arch = ?;

-- Find all packages that provide a capability.
-- name: LookupProviders :many
SELECT DISTINCT p.id, p.name, p.epoch, p.version, p.release, p.arch, p.format, p.install_time, p.summary
FROM packages p
JOIN caps c ON p.id = c.package_id
WHERE c.kind = 'provide' AND c.name = ?
ORDER BY p.name ASC;

-- Get all files for a package.
-- name: FilesByPackage :many
SELECT package_id, path, mode, is_dir, is_symlink, link_target, sha256
FROM files
WHERE package_id = ?
ORDER BY path ASC;

-- Count total installed packages.
-- name: CountPackages :one
SELECT COUNT(*) FROM packages;

-- Check if a package is installed by name.
-- name: IsInstalledByName :one
SELECT EXISTS(SELECT 1 FROM packages WHERE name = ?);

-- Get all capabilities for a package.
-- name: CapsByPackage :many
SELECT kind, name, flags, version
FROM caps
WHERE package_id = ?
ORDER BY kind ASC, name ASC;
