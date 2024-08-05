-- -*- sql-dialect: sqlite -*-

CREATE TABLE IF NOT EXISTS users (
       id INTEGER PRIMARY KEY,
       username TEXT,
       salt TEXT,
       hash TEXT
);
