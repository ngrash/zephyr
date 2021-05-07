package main

import (
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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		type Run struct {
			Id     string
			Status string
		}

		type PipelineViewModel struct {
			config.Pipeline
			NextRun *time.Time
			LastRun *time.Time
			History []Run
		}

		vms := make([]PipelineViewModel, len(pipelines))
		for i, p := range pipelines {
			var nextRun *time.Time
			if schedJob, ok := scheduled[p.Name]; ok {
				t := schedJob.ScheduledTime()
				nextRun = &t
			}

			var lastRun *time.Time
			var latest []database.Pipeline
			db.Select(&latest, database.SelectLatestPipelinesByName, p.Name)
			if len(latest) > 0 {
				lastRun = &(latest[0].CreatedAt)
			}

			history := make([]Run, len(latest))
			for i := 0; i < len(latest); i++ {
				r := latest[len(latest)-(1+i)]
				history[i] = Run{
					Id:     r.Id,
					Status: r.Status.String(),
				}
			}

			vms[i] = PipelineViewModel{p, nextRun, lastRun, history}
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
		}{
			def,
			&pipeline,
			jobs,
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
		}{
			&pipeline,
			&job,
			logLines,
		}
		tmpl := template.Must(template.ParseFiles("templates/job_instance.html", "templates/base.layout.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	log.Print("serving")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
