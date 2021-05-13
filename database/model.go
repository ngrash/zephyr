package database

import (
	"time"

	"github.com/ngrash/zephyr/stdstreams"
)

type Status uint8

const (
	StatusPending Status = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusCancelled
)

func (i Status) String() string {
	switch i {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return "<unknown>"
	}
}

type Pipeline struct {
	Id        string    `db:"id"`
	Name      string    `db:"name"`
	Status    Status    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const (
	CreatePipeline                 = "INSERT INTO pipelines (id, name, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)"
	UpdatePipelineStatusById       = "UPDATE pipelines SET status = ?, updated_at = ? WHERE id = ?"
	GetPipelineById                = "SELECT * FROM pipelines WHERE id = ?"
	GetLastFailedPipelineByName    = "SELECT * FROM pipelines WHERE name = ? AND status = 3 ORDER BY updated_at DESC LIMIT 1"
	GetLastCompletedPipelineByName = "SELECT * FROM pipelines WHERE name = ? AND status = 2 ORDER BY updated_at DESC LIMIT 1"
	GetLastStatusByPipelineName    = "SELECT status FROM pipelines WHERE name = ? ORDER BY updated_at DESC LIMIT 1"
)

type Job struct {
	Id         int64
	Name       string    `db:"name"`
	Command    string    `db:"command"`
	Status     Status    `db:"status"`
	PipelineId string    `db:"pipeline_id"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

const (
	CreateJob                 = "INSERT INTO jobs (name, command, pipeline_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"
	UpdateJobStatusById       = "UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?"
	SelectJobsByPipelineId    = "SELECT * FROM jobs WHERE pipeline_id = ?"
	GetJobByNameAndPipelineId = "SELECT * FROM jobs WHERE name = ? AND pipeline_id = ? "
)

type Log struct {
	Stream   stdstreams.Stream
	Line     string    `db:"line"`
	LoggedAt time.Time `db:"logged_at"`
}

const (
	CreateLog = "INSERT INTO logs (stream, job_id, line, logged_at) VALUES (?, ?, ?, ?)"

	SelectLogsByJobNameAndPipelineId = "SELECT stream, line, logged_at FROM logs JOIN jobs ON jobs.id = logs.job_id WHERE jobs.name = ? AND jobs.pipeline_id = ? ORDER BY logged_at ASC"
)
