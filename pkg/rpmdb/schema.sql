-- RPM 4.16+ SQLite schema (Fedora, RHEL, Rocky, Alma, openSUSE).
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
