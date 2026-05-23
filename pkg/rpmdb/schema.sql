-- Full RPM 4.16+ SQLite schema (Fedora 38+, RHEL 9+, openSUSE Tumbleweed)
-- Canonical schema used by both readers and writers.

CREATE TABLE Packages (
    hnum INTEGER NOT NULL PRIMARY KEY,
    blob BLOB NOT NULL
);

CREATE TABLE Name (
    name TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum)
);

CREATE INDEX nameindex ON Name (name ASC);

CREATE TABLE Providename (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX providenameindex ON Providename (key ASC);

CREATE TABLE Requirename (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX requirenameindex ON Requirename (key ASC);

CREATE TABLE Conflictname (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX conflictnameindex ON Conflictname (key ASC);

CREATE TABLE Obsoletename (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX obsoletenameindex ON Obsoletename (key ASC);

CREATE TABLE Basenames (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX basenamesindex ON Basenames (key ASC);

CREATE TABLE Dirnames (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX dirnamesindex ON Dirnames (key ASC);

CREATE TABLE Filedigests (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX filedigestsindex ON Filedigests (key ASC);

CREATE TABLE Triggername (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX triggernameindex ON Triggername (key ASC);

CREATE TABLE Sha1header (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX sha1headerindex ON Sha1header (key ASC);

CREATE TABLE Installtid (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX installtidindex ON Installtid (key ASC);

CREATE TABLE Sigmd5 (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX sigmd5index ON Sigmd5 (key ASC);
