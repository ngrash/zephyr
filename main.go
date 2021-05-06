package main

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/stdstreams"

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

		type PipelineViewModel struct {
			config.Pipeline
			NextRun *time.Time
			LastRun *time.Time
		}

		vms := make([]PipelineViewModel, len(pipelines))
		for i, p := range pipelines {
			var nextRun *time.Time
			if schedJob, ok := scheduled[p.Name]; ok {
				t := schedJob.ScheduledTime()
				nextRun = &t
			}

			vms[i] = PipelineViewModel{p, nextRun, nil}
		}

		data := struct{ Pipelines []PipelineViewModel }{vms}
		tmpl := template.Must(template.ParseFiles("templates/index.html"))
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

		var pipeline PipelineInstance
		err := db.Get(&pipeline, "SELECT * FROM pipelines WHERE id = ?", id)
		if err != nil {
			log.Fatal(err)
		}
		def := pipelineByName(pipeline.Name)

		var jobs []JobInstance
		err = db.Select(&jobs, "SELECT * FROM jobs WHERE pipeline_id = ?", id)
		if err != nil {
			log.Fatal(err)
		}

		data := struct {
			Def      *config.Pipeline
			Instance *PipelineInstance
			Jobs     []JobInstance
		}{
			def,
			&pipeline,
			jobs,
		}
		tmpl := template.Must(template.ParseFiles("templates/pipeline_instance.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	http.HandleFunc("/job_instance", func(w http.ResponseWriter, r *http.Request) {
		pipelineId := r.FormValue("pipeline_id")
		jobName := r.FormValue("name")

		var pipeline PipelineInstance
		err := db.Get(&pipeline, "SELECT * FROM pipelines WHERE id = ?", pipelineId)
		if err != nil {
			log.Fatal(err)
		}

		var job JobInstance
		err = db.Get(&job, "SELECT * FROM jobs WHERE pipeline_id = ? AND name = ?", pipelineId, jobName)
		if err != nil {
			log.Fatal(err)
		}

		type LogLine struct {
			Stream   stdstreams.Stream
			Line     string
			LoggedAt time.Time `db:"logged_at"`
		}

		var logLines []LogLine
		db.Select(&logLines, "SELECT stream, line, logged_at FROM logs JOIN jobs ON jobs.id = logs.job_id WHERE jobs.name = ? AND jobs.pipeline_id = ? ORDER BY logged_at ASC", jobName, pipelineId)

		data := struct {
			Pipe *PipelineInstance
			Job  *JobInstance
			Log  []LogLine
		}{
			&pipeline,
			&job,
			logLines,
		}
		tmpl := template.Must(template.ParseFiles("templates/job_instance.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	log.Print("serving")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
