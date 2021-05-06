package main

import (
	"log"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/database"
	"github.com/ngrash/zephyr/stdstreams"
)

type Executor struct {
	db *sqlx.DB
}

func NewExecutor(db *sqlx.DB) *Executor {
	return &Executor{db}
}

func (e *Executor) Run(def *config.Pipeline) string {
	log.Printf("Executor.Run({Name: %s})", def.Name)

	instance := &database.Pipeline{
		Id:     uuid.NewString(),
		Name:   def.Name,
		Status: database.StatusPending,
	}
	now := time.Now().UTC()
	e.db.MustExec(database.CreatePipeline,
		instance.Id,
		instance.Name,
		instance.Status,
		now,
		now)

	jis := make([]*database.Job, len(def.Jobs))
	for i, j := range def.Jobs {
		job := &database.Job{Status: database.StatusPending, Name: j.Name, Command: j.Command}
		result := e.db.MustExec(database.CreateJob,
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

func (e *Executor) AsyncPipelineRoutine(p *database.Pipeline, js []*database.Job) {
	e.setPipelineStatus(p, database.StatusRunning)

	for idx, job := range js {
		e.setJobStatus(job, database.StatusRunning)

		streams := stdstreams.NewLogWithCallback(func(newLine *stdstreams.Line) {
			e.db.MustExec(database.CreateLog,
				newLine.Stream,
				job.Id,
				newLine.Text,
				newLine.Time.UTC())
		})

		cmd := exec.Command("sh", "-c", job.Command)
		cmd.Stderr = streams.Stderr()
		cmd.Stdout = streams.Stdout()

		if err := cmd.Run(); err != nil {
			e.setPipelineStatus(p, database.StatusFailed)
			e.setJobStatus(job, database.StatusFailed)

			for j := idx + 1; j < len(js); j++ {
				e.setJobStatus(js[j], database.StatusCancelled)
			}

			return
		}

		e.setJobStatus(job, database.StatusCompleted)
	}

	e.setPipelineStatus(p, database.StatusCompleted)
}

func (e *Executor) setPipelineStatus(p *database.Pipeline, st database.Status) {
	p.Status = st
	p.UpdatedAt = time.Now().UTC()
	e.db.MustExec(database.UpdatePipelineStatusById, p.Status, p.UpdatedAt, p.Id)
}

func (e *Executor) setJobStatus(j *database.Job, st database.Status) {
	j.Status = st
	j.UpdatedAt = time.Now().UTC()
	e.db.MustExec(database.UpdateJobStatusById, j.Status, j.UpdatedAt, j.Id)
}
