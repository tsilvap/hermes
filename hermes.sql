-- -*- sql-dialect: sqlite -*-

CREATE TABLE IF NOT EXISTS users (
       id INTEGER PRIMARY KEY,
       username TEXT,
       salt TEXT,
       hash TEXT
);

CREATE TABLE IF NOT EXISTS uploaded_files (
       id INTEGER PRIMARY KEY,
       title TEXT,
       uploader TEXT,
       file_path TEXT,
       created_at INTEGER
);
