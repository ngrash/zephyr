package zephyr

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/database"
	"github.com/ngrash/zephyr/stdstreams"
	"log"
	"time"
)

type Repository struct {
	db     *sqlx.DB
	mailer Mailer
}

func NewRepository(db *sqlx.DB, mailer Mailer) Repository {
	return Repository{db, mailer}
}

func (r Repository) CreatePipeline(pipeline config.Pipeline) PipelineInstanceId {
	pId := PipelineInstanceId(uuid.NewString())
	now := time.Now().UTC()
	r.db.MustExec(database.CreatePipeline,
		pId,
		pipeline.Name,
		database.StatusPending,
		now,
		now)
	return pId
}

func (r Repository) StartPipeline(_ config.Pipeline, pId PipelineInstanceId) {
	r.setPipelineStatus(pId, database.StatusRunning)
}

func (r Repository) CreateJob(_ config.Pipeline, pipelineId PipelineInstanceId, job config.Job) JobInstanceId {
	now := time.Now().UTC()
	j := &database.Job{Status: database.StatusPending, Name: job.Name, Command: job.Command}
	result := r.db.MustExec(database.CreateJob,
		job.Name,
		job.Command,
		pipelineId,
		j.Status,
		now,
		now)
	id, err := result.LastInsertId()
	if err != nil {
		panic(err)
	}
	return JobInstanceId(id)
}

func (r Repository) StartJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusRunning)
}

func (r Repository) LogJobOutput(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId, line stdstreams.Line) {
	r.db.MustExec(database.CreateLog,
		line.Stream,
		jId,
		line.Text,
		line.Time.UTC())
}

func (r Repository) FailJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusFailed)
}

func (r Repository) CompleteJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusCompleted)
}

func (r Repository) CancelJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusCancelled)
}

func (r Repository) FailPipeline(pipeline config.Pipeline, pId PipelineInstanceId) {
	r.setPipelineStatus(pId, database.StatusFailed)

	if pipeline.Alert != "" {
		go func() {
			err := r.mailer.Send(
				pipeline.Alert,
				fmt.Sprintf("Pipeline failed: %s", pipeline.Name),
				fmt.Sprintf("A pipeline failed and was configured to alert you: __BASE_URL__/pipeline_instance?id=%s", pId),
			)
			if err != nil {
				log.Printf("sending mail: %s", err)
			}
		}()
	}
}

func (r Repository) CompletePipeline(_ config.Pipeline, pId PipelineInstanceId) {
	r.setPipelineStatus(pId, database.StatusCompleted)
}

func (r Repository) setPipelineStatus(pId PipelineInstanceId, s database.Status) {
	r.db.MustExec(database.UpdatePipelineStatusById, s, time.Now().UTC(), pId)
}

func (r Repository) setJobStatus(jId JobInstanceId, s database.Status) {
	r.db.MustExec(database.UpdateJobStatusById, s, time.Now().UTC(), jId)
}
