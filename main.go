package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/database"

	_ "github.com/mattn/go-sqlite3"
)

var scheduled map[string]*gocron.Job

func main() {

	db, err := sqlx.Connect("sqlite3", "zephyr.db?foreign_keys=on")
	if err != nil {
		log.Fatal(err)
	}
	MigrateSchema(db)

	executor := NewExecutor(db)

	pipelines, err := config.LoadPipelines("pipelines.yaml")

	scheduler := gocron.NewScheduler(time.UTC)
	scheduled = make(map[string]*gocron.Job)
	for _, p := range pipelines {
		pipeline := p
		if p.Schedule != "" {
			job, err := scheduler.Cron(p.Schedule).Do(func() {
				executor.Run(&pipeline)
			})
			if err != nil {
				log.Fatal(err)
			}
			scheduled[p.Name] = job
		}
	}
	scheduler.StartAsync()

	pipelineByName := func(name string) *config.Pipeline {
		for _, p := range pipelines {
			if p.Name == name {
				return &p
			}
		}
		return nil
	}

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		type PipelineViewModel struct {
			config.Pipeline
			LastStatus      string
			NextRunAt       string
			LastCompleted   *database.Pipeline
			LastCompletedAt string
			LastFailed      *database.Pipeline
			LastFailedAt    string
		}

		vms := make([]PipelineViewModel, len(pipelines))
		for i, p := range pipelines {

			pvm := PipelineViewModel{p, "N/A", "N/A", nil, "N/A", nil, "N/A"}

			if schedJob, ok := scheduled[p.Name]; ok {
				pvm.NextRunAt = schedJob.ScheduledTime().Format(time.RFC3339)
			}

			var err error

			var lastStatus uint8
			err = db.Get(&lastStatus, database.GetLastStatusByPipelineName, p.Name)
			if err == nil {
				pvm.LastStatus = database.Status(lastStatus).String()
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
		tmpl := template.Must(template.ParseFiles("templates/index.html", "templates/base.layout.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		pipeline := pipelineByName(name)
		id := executor.Run(pipeline)
		http.Redirect(w, r, "pipeline_instance?id="+id, http.StatusSeeOther)
	})

	http.HandleFunc("/pipeline_instance", func(w http.ResponseWriter, r *http.Request) {
		id := r.FormValue("id")

		var pipeline database.Pipeline
		err := db.Get(&pipeline, database.GetPipelineById, id)
		if err != nil {
			log.Fatal(err)
		}
		def := pipelineByName(pipeline.Name)

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
		tmpl := template.Must(template.ParseFiles("templates/pipeline_instance.html", "templates/base.layout.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	http.HandleFunc("/job_instance", func(w http.ResponseWriter, r *http.Request) {
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
		tmpl := template.Must(template.ParseFiles("templates/job_instance.html", "templates/base.layout.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	log.Print("serving")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
