package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/database"
)

var tmplFuncs = template.FuncMap{
	"bsColor": func(s database.Status) string {
		switch s {
		case database.StatusPending:
			return "primary"
		case database.StatusRunning:
			return "info"
		case database.StatusCompleted:
			return "success"
		case database.StatusFailed:
			return "danger"
		case database.StatusCancelled:
			return "warning"
		default:
			return "secondary"
		}
	},
	"indexPath":    indexPath,
	"runPath":      runPath,
	"pipelinePath": pipelinePath,
	"jobPath":      jobPath,
}

const (
	indexPattern    = "/"
	runPattern      = "/run"
	pipelinePattern = "/pipeline_instance"
	jobPattern      = "/job_instance"
)

func indexPath() string             { return indexPattern }
func runPath() string               { return fmt.Sprintf("%s", runPattern) }
func pipelinePath(id string) string { return fmt.Sprintf("%s?id=%s", pipelinePattern, id) }
func jobPath(name, pipeline_id string) string {
	return fmt.Sprintf("%s?name=%s&pipeline_id=%s", jobPattern, name, pipeline_id)
}

func indexHandler(pipelines []config.Pipeline, db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		type PipelineViewModel struct {
			config.Pipeline
			LastStatus      *database.Status
			NextRunAt       string
			LastCompleted   *database.Pipeline
			LastCompletedAt string
			LastFailed      *database.Pipeline
			LastFailedAt    string
		}

		vms := make([]PipelineViewModel, len(pipelines))
		for i, p := range pipelines {

			pvm := PipelineViewModel{p, nil, "N/A", nil, "N/A", nil, "N/A"}

			if schedJob, ok := scheduled[p.Name]; ok {
				pvm.NextRunAt = schedJob.ScheduledTime().Format(time.RFC3339)
			}

			var err error

			var lastStatus uint8
			err = db.Get(&lastStatus, database.GetLastStatusByPipelineName, p.Name)
			if err == nil {
				st := database.Status(lastStatus)
				pvm.LastStatus = &st
			} else if err != sql.ErrNoRows {
				log.Fatal(err)
			}

			var lastCompleted database.Pipeline
			err = db.Get(&lastCompleted, database.GetLastCompletedPipelineByName, p.Name)
			if err == nil {
				pvm.LastCompleted = &lastCompleted
				pvm.LastCompletedAt = lastCompleted.UpdatedAt.Format(time.RFC3339)
			} else if err != sql.ErrNoRows {
				log.Fatal(err)
			}

			var lastFailed database.Pipeline
			err = db.Get(&lastFailed, database.GetLastFailedPipelineByName, p.Name)
			if err == nil {
				pvm.LastFailed = &lastFailed
				pvm.LastFailedAt = lastFailed.UpdatedAt.Format(time.RFC3339)
			} else if err != sql.ErrNoRows {
				log.Fatal(err)
			}

			vms[i] = pvm
		}

		data := struct{ Pipelines []PipelineViewModel }{vms}
		tmpl := template.Must(
			template.New("index.page.html").
				Funcs(tmplFuncs).
				ParseFiles("templates/index.page.html", "templates/base.layout.html"),
		)
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	}
}

func runHandler(pipelines []config.Pipeline, executor *Executor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		pipeline := pipelineByName(pipelines, name)
		id := executor.Run(pipeline)
		http.Redirect(w, r, pipelinePath(id), http.StatusSeeOther)
	}
}

func pipelineHandler(pipelines []config.Pipeline, db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.FormValue("id")

		var pipeline database.Pipeline
		err := db.Get(&pipeline, database.GetPipelineById, id)
		if err != nil {
			log.Fatal(err)
		}
		def := pipelineByName(pipelines, pipeline.Name)

		var jobs []database.Job
		err = db.Select(&jobs, database.SelectJobsByPipelineId, id)
		if err != nil {
			log.Fatal(err)
		}

		data := struct {
			Def      *config.Pipeline
			Instance *database.Pipeline
			Jobs     []database.Job
			Date     string
		}{
			def,
			&pipeline,
			jobs,
			pipeline.UpdatedAt.Format(time.RFC3339),
		}
		tmpl := template.Must(
			template.New("pipeline.page.html").
				Funcs(tmplFuncs).
				ParseFiles("templates/pipeline.page.html", "templates/base.layout.html"),
		)
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	}
}

func jobHandler(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pipelineId := r.FormValue("pipeline_id")
		jobName := r.FormValue("name")

		var pipeline database.Pipeline
		err := db.Get(&pipeline, database.GetPipelineById, pipelineId)
		if err != nil {
			log.Fatal(err)
		}

		var job database.Job
		err = db.Get(&job, database.GetJobByNameAndPipelineId, jobName, pipelineId)
		if err != nil {
			log.Fatal(err)
		}

		var logLines []database.Log
		db.Select(&logLines, database.SelectLogsByJobNameAndPipelineId, jobName, pipelineId)

		data := struct {
			Pipe *database.Pipeline
			Job  *database.Job
			Log  []database.Log
			Date string
		}{
			&pipeline,
			&job,
			logLines,
			job.UpdatedAt.Format(time.RFC3339),
		}
		tmpl := template.Must(
			template.New("job.page.html").
				Funcs(tmplFuncs).
				ParseFiles("templates/job.page.html", "templates/base.layout.html"),
		)
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	}
}

func pipelineByName(pl []config.Pipeline, name string) *config.Pipeline {
	for _, p := range pl {
		if p.Name == name {
			return &p
		}
	}
	return nil
}
