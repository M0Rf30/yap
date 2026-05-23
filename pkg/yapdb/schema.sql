-- YAP installed package registry (per rootDir).
-- Schema version 1: stable.

CREATE TABLE packages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    epoch       TEXT NOT NULL DEFAULT '',     -- "" if not set
    version     TEXT NOT NULL,
    release     TEXT NOT NULL,
    arch        TEXT NOT NULL,
    format      TEXT NOT NULL,                -- "rpm" | "deb" | "apk" | "pacman"
    install_time INTEGER NOT NULL,            -- unix seconds
    summary     TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX packages_name_arch ON packages (name, arch);

-- Files placed on disk, for uninstall + conflict detection.
CREATE TABLE files (
    package_id  INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    mode        INTEGER NOT NULL,
    is_dir      INTEGER NOT NULL DEFAULT 0,
    is_symlink  INTEGER NOT NULL DEFAULT 0,
    link_target TEXT NOT NULL DEFAULT '',
    sha256      TEXT NOT NULL DEFAULT ''      -- hex, '' for dirs/symlinks
);
CREATE INDEX files_package ON files (package_id);
CREATE INDEX files_path ON files (path);

-- Capabilities (provides + requires + obsoletes + conflicts).
CREATE TABLE caps (
    package_id  INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,                -- "provide" | "require" | "obsolete" | "conflict"
    name        TEXT NOT NULL,
    flags       INTEGER NOT NULL DEFAULT 0,   -- comparison flags (>=, <, ==, etc.)
    version     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX caps_name ON caps (name);
CREATE INDEX caps_package ON caps (package_id);

-- Schema version for forward compat.
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO meta (key, value) VALUES ('schema_version', '1');
