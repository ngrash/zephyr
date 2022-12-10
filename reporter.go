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

type Reporter struct {
	db     *sqlx.DB
	mailer Mailer
}

func NewReporter(db *sqlx.DB, mailer Mailer) Reporter {
	return Reporter{db, mailer}
}

func (r Reporter) PipelineCreated(pipeline config.Pipeline) PipelineInstanceId {
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

func (r Reporter) PipelineStarted(pipeline config.Pipeline, pId PipelineInstanceId) {
	r.setPipelineStatus(pId, database.StatusRunning)
}

func (r Reporter) JobCreated(pipeline config.Pipeline, pipelineId PipelineInstanceId, job config.Job) JobInstanceId {
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

func (r Reporter) JobStarted(pipeline config.Pipeline, job config.Job, id JobInstanceId) {
	//TODO implement me
	panic("implement me")
}

func (r Reporter) JobOutput(pipeline config.Pipeline, job config.Job, id JobInstanceId, line stdstreams.Line) {
	//TODO implement me
	panic("implement me")
}

func (r Reporter) JobFailed(pipeline config.Pipeline, job config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusFailed)
}

func (r Reporter) JobCompleted(pipeline config.Pipeline, job config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusCompleted)
}

func (r Reporter) JobCancelled(pipeline config.Pipeline, job config.Job, jId JobInstanceId) {
	r.setJobStatus(jId, database.StatusCancelled)
}

func (r Reporter) PipelineFailed(pipeline config.Pipeline, pId PipelineInstanceId) {
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

func (r Reporter) PipelineCompleted(pipeline config.Pipeline, pId PipelineInstanceId) {
	r.setPipelineStatus(pId, database.StatusCompleted)
}

func (r Reporter) setPipelineStatus(pId PipelineInstanceId, s database.Status) {
	r.db.MustExec(database.UpdatePipelineStatusById, s, time.Now().UTC(), pId)
}

func (r Reporter) setJobStatus(jId JobInstanceId, s database.Status) {
	r.db.MustExec(database.UpdateJobStatusById, s, time.Now().UTC(), jId)
}
