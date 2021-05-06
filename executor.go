package main

import (
	"log"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/stdstreams"
)

type InstanceStatus uint8

const (
	StatusPending InstanceStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusCancelled
)

func (i InstanceStatus) String() string {
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

type PipelineInstance struct {
	Id        string         `db:"id"`
	Name      string         `db:"name"`
	Status    InstanceStatus `db:"status"`
	CreatedAt time.Time      `db:"created_at"`
	UpdatedAt time.Time      `db:"updated_at"`
}

func (p *PipelineInstance) UpdateStatus(db *sqlx.DB, s InstanceStatus) {
	p.Status = s
	db.MustExec("UPDATE pipelines SET status = ?, updated_at = ? WHERE id = ?", p.Status, time.Now().UTC(), p.Id)
}

type JobInstance struct {
	Id         int64
	Name       string         `db:"name"`
	Command    string         `db:"command"`
	Status     InstanceStatus `db:"status"`
	PipelineId string         `db:"pipeline_id"`
	CreatedAt  time.Time      `db:"created_at"`
	UpdatedAt  time.Time      `db:"updated_at"`
}

func (j *JobInstance) UpdateStatus(db *sqlx.DB, s InstanceStatus) {
	j.Status = s
	db.MustExec("UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?", j.Status, time.Now().UTC(), j.Id)
}

type Executor struct {
	db *sqlx.DB
}

func NewExecutor(db *sqlx.DB) *Executor {
	return &Executor{db}
}

func (e *Executor) Run(def *config.Pipeline) string {
	log.Printf("Executor.Run({Name: %s})", def.Name)

	instance := &PipelineInstance{
		Id:     uuid.NewString(),
		Name:   def.Name,
		Status: StatusPending,
	}
	now := time.Now().UTC()
	e.db.MustExec("INSERT INTO pipelines (id, name, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		instance.Id,
		instance.Name,
		instance.Status,
		now,
		now)

	jis := make([]*JobInstance, len(def.Jobs))
	for i, j := range def.Jobs {
		job := &JobInstance{Status: StatusPending, Name: j.Name, Command: j.Command}
		result := e.db.MustExec("INSERT INTO jobs (name, command, pipeline_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			job.Name,
			job.Command,
			instance.Id,
			job.Status,
			now,
			now)
		id, _ := result.LastInsertId()
		job.Id = id
		jis[i] = job
	}

	go e.AsyncPipelineRoutine(instance, jis)

	return instance.Id
}

func (e *Executor) AsyncPipelineRoutine(p *PipelineInstance, js []*JobInstance) {
	p.UpdateStatus(e.db, StatusRunning)

	for idx, job := range js {
		job.UpdateStatus(e.db, StatusRunning)

		streams := stdstreams.NewLogWithCallback(func(newLine *stdstreams.Line) {
			e.db.MustExec("INSERT INTO logs (stream, job_id, line, logged_at) VALUES (?, ?, ?, ?)",
				newLine.Stream,
				job.Id,
				newLine.Text,
				newLine.Time.UTC())
		})

		cmd := exec.Command("sh", "-c", job.Command)
		cmd.Stderr = streams.Stderr()
		cmd.Stdout = streams.Stdout()

		if err := cmd.Run(); err != nil {
			job.UpdateStatus(e.db, StatusFailed)
			p.UpdateStatus(e.db, StatusFailed)

			for j := idx + 1; j < len(js); j++ {
				js[j].UpdateStatus(e.db, StatusCancelled)
			}

			return
		}

		job.UpdateStatus(e.db, StatusCompleted)
	}

	p.UpdateStatus(e.db, StatusCompleted)
}
