package main

import "github.com/jmoiron/sqlx"

func MigrateSchema(db *sqlx.DB) {
	db.MustExec(`CREATE TABLE IF NOT EXISTS pipelines (
		id         TEXT PRIMARY KEY,
		handle     TEXT     NOT NULL,
		status     TEXT     NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	) WITHOUT ROWID`)

	db.MustExec(`CREATE TABLE IF NOT EXISTS jobs (
		id          INTEGER  PRIMARY KEY,
		pipeline_id TEXT     NOT NULL REFERENCES pipelines(id),
		handle      TEXT     NOT NULL,
		status      TEXT     NOT NULL,
		created_at  DATETIME NOT NULL,
		updated_at  DATETIME NOT NULL
	)`)

	db.MustExec(`CREATE TABLE IF NOT EXISTS logs (
		id         INTEGER PRIMARY KEY,
		job_id     INTEGER  NOT NULL REFERENCES jobs(id),
		stream     INTEGER  NOT NULL,
		line       TEXT     NOT NULL,
		logged_at  DATETIME NOT NULL
	)`)
}
