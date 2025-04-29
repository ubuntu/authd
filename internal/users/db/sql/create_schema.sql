CREATE TABLE IF NOT EXISTS users (
    name      TEXT NOT NULL,  -- Uniqueness is enforced by the index below
    uid       INT PRIMARY KEY, -- Uniqueness and not NULL is enforced by PRIMARY KEY
    gid       INT NOT NULL,
    gecos     TEXT DEFAULT "",
    dir       TEXT DEFAULT "",
    shell     TEXT DEFAULT "/bin/bash",
    broker_id TEXT DEFAULT ""
);
CREATE UNIQUE INDEX "idx_user_name" ON users ("name");

CREATE TABLE IF NOT EXISTS GROUPS (
    name TEXT NOT NULL,  -- Uniqueness is enforced by the index below
    gid  INT PRIMARY KEY, -- Uniqueness and not NULL is enforced by PRIMARY KEY
    ugid INT NOT NULL    -- Uniqueness is enforced by the index below
);
CREATE UNIQUE INDEX "idx_group_name" ON GROUPS ("name");
CREATE UNIQUE INDEX "idx_group_ugid" ON GROUPS ("ugid");

CREATE TABLE IF NOT EXISTS users_to_groups (
    uid INT NOT NULL,
    gid INT NOT NULL,
    PRIMARY KEY (uid, gid),
    FOREIGN KEY (uid) REFERENCES users (uid) ON DELETE CASCADE,
    FOREIGN KEY (gid) REFERENCES GROUPS (gid) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS users_to_local_groups (
    uid        INT NOT NULL,
    group_name TEXT NOT NULL,
    PRIMARY KEY (uid, group_name),
    FOREIGN KEY (uid) REFERENCES users (uid) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS schema_version (
    version INT PRIMARY KEY
);
