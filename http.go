package zephyr

import (
	"database/sql"
	"fmt"
	"github.com/go-co-op/gocron"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/database"
)

type Server struct {
	mux *http.ServeMux
}

func NewServer(pipelines config.Pipelines, db *sqlx.DB, executor *Executor, jobs map[string]*gocron.Job) Server {
	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))
	mux.HandleFunc("/", indexHandler(pipelines, db, jobs))
	mux.HandleFunc("/run", runHandler(pipelines, executor))
	mux.HandleFunc("/pipeline_instance", pipelineInstanceHandler(pipelines, db))
	mux.HandleFunc("/job_instance", jobInstanceHandler(db))

	return Server{mux}
}

func (s Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

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
	"indexPath":            indexPath,
	"runPath":              runPath,
	"pipelineInstancePath": pipelineInstancePath,
	"jobInstancePath":      jobInstancePath,
}

const (
	indexPattern            = "/"
	runPattern              = "/run"
	pipelineInstancePattern = "/pipeline_instance"
	jobInstancePattern      = "/job_instance"
)

func indexPath() string { return indexPattern }
func runPath() string   { return fmt.Sprintf("%s", runPattern) }
func pipelineInstancePath(id string) string {
	return fmt.Sprintf("%s?id=%s", pipelineInstancePattern, id)
}
func jobInstancePath(name, pipeline_id string) string {
	return fmt.Sprintf("%s?name=%s&pipeline_id=%s", jobInstancePattern, name, pipeline_id)
}

func indexHandler(pipelines []config.Pipeline, db *sqlx.DB, jobs map[string]*gocron.Job) http.HandlerFunc {
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

			if schedJob, ok := jobs[p.Name]; ok {
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
		http.Redirect(w, r, pipelineInstancePath(id), http.StatusSeeOther)
	}
}

func pipelineInstanceHandler(pipelines []config.Pipeline, db *sqlx.DB) http.HandlerFunc {
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
			template.New("pipeline_instance.page.html").
				Funcs(tmplFuncs).
				ParseFiles("templates/pipeline_instance.page.html", "templates/base.layout.html"),
		)
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	}
}

func jobInstanceHandler(db *sqlx.DB) http.HandlerFunc {
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
			template.New("job_instance.page.html").
				Funcs(tmplFuncs).
				ParseFiles("templates/job_instance.page.html", "templates/base.layout.html"),
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
