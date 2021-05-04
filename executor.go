package main

import (
	"log"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
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
	Id        string              `db:"id"`
	Handle    string              `db:"handle"`
	Def       *PipelineDefinition `db:"-"`
	Status    InstanceStatus      `db:"status"`
	Jobs      []*JobInstance      `db:"-"`
	CreatedAt time.Time           `db:"created_at"`
	UpdatedAt time.Time           `db:"updated_at"`
}

func (p *PipelineInstance) UpdateStatus(db *sqlx.DB, s InstanceStatus) {
	p.Status = s
	db.MustExec("UPDATE pipelines SET status = ?, updated_at = ? WHERE id = ?", p.Status, time.Now().UTC(), p.Id)
}

type JobInstance struct {
	Id         int64
	Handle     string          `db:"handle"`
	Def        JobDefinition   `db:"-"`
	Status     InstanceStatus  `db:"status"`
	Log        *stdstreams.Log `db:"-"`
	PipelineId string          `db:"pipeline_id"`
	CreatedAt  time.Time       `db:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at"`
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

func (e *Executor) Run(def *PipelineDefinition) string {
	log.Printf("Executor.Run({Handle: %s})", def.Handle)

	instance := &PipelineInstance{
		Id:     uuid.NewString(),
		Handle: def.Handle,
		Def:    def,
		Status: StatusPending,
		Jobs:   make([]*JobInstance, len(def.Jobs)),
	}
	now := time.Now().UTC()
	e.db.MustExec("INSERT INTO pipelines (id, handle, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		instance.Id,
		instance.Handle,
		instance.Status,
		now,
		now)

	for i, j := range def.Jobs {
		job := &JobInstance{Def: j, Status: StatusPending, Handle: j.Handle}
		instance.Jobs[i] = job
		result := e.db.MustExec("INSERT INTO jobs (handle, pipeline_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			job.Handle,
			instance.Id,
			job.Status,
			now,
			now)
		id, _ := result.LastInsertId()
		job.Id = id
	}

	go e.AsyncPipelineRoutine(instance)

	return instance.Id
}

func (e *Executor) AsyncPipelineRoutine(i *PipelineInstance) {
	i.UpdateStatus(e.db, StatusRunning)

	for idx, job := range i.Jobs {
		job.UpdateStatus(e.db, StatusRunning)

		job.Log = stdstreams.NewLogWithCallback(func(newLine *stdstreams.Line) {
			e.db.MustExec("INSERT INTO logs (stream, job_id, line, logged_at) VALUES (?, ?, ?, ?)",
				newLine.Stream,
				job.Id,
				newLine.Text,
				newLine.Time.UTC())
		})

		cmd := exec.Command("sh", "-c", job.Def.Command)
		cmd.Stderr = job.Log.Stderr()
		cmd.Stdout = job.Log.Stdout()

		if err := cmd.Run(); err != nil {
			job.UpdateStatus(e.db, StatusFailed)
			i.UpdateStatus(e.db, StatusFailed)

			for j := idx + 1; j < len(i.Jobs); j++ {
				i.Jobs[j].UpdateStatus(e.db, StatusCancelled)
			}

			return
		}

		job.UpdateStatus(e.db, StatusCompleted)
	}

	i.UpdateStatus(e.db, StatusCompleted)
}
