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

type PipelineDefinition struct {
	Handle   string
	Schedule string
	Jobs     []JobDefinition

	schedulerJob *gocron.Job
}

type JobDefinition struct {
	Handle  string
	Command string
}

type PipelineViewModel struct {
	*PipelineDefinition
	NextRun *time.Time
	LastRun *time.Time
}

func main() {

	db, err := sqlx.Connect("sqlite3", "zephyr.db?foreign_keys=on")
	if err != nil {
		log.Fatal(err)
	}
	MigrateSchema(db)

	executor := NewExecutor(db)

	ps, err := config.LoadPipelines("pipelines.yaml")
	pipelines := make([]*PipelineDefinition, len(ps))
	for i, p := range ps {
		jobs := make([]JobDefinition, len(p.Jobs))
		for ii, j := range p.Jobs {
			jobs[ii] = JobDefinition{Handle: j.Name, Command: j.Command}
		}
		pipelines[i] = &PipelineDefinition{
			Handle: p.Name,
			Jobs:   jobs,
		}
	}

	scheduler := gocron.NewScheduler(time.UTC)
	for _, p := range pipelines {
		pipeline := p
		if p.Schedule != "" {
			job, err := scheduler.Cron(p.Schedule).Do(func() {
				executor.Run(pipeline)
			})
			if err != nil {
				log.Fatal(err)
			}
			p.schedulerJob = job
		}
	}
	scheduler.StartAsync()

	pipelineByHandle := func(handle string) *PipelineDefinition {
		var pipeline *PipelineDefinition
		for _, p := range pipelines {
			if p.Handle == handle {
				pipeline = p
				break
			}
		}
		return pipeline
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		vms := make([]PipelineViewModel, len(pipelines))
		for i, p := range pipelines {
			var nextRun *time.Time
			if p.schedulerJob != nil {
				t := p.schedulerJob.ScheduledTime()
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
		handle := r.FormValue("handle")
		pipeline := pipelineByHandle(handle)
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
		def := pipelineByHandle(pipeline.Handle)

		var jobs []JobInstance
		err = db.Select(&jobs, "SELECT * FROM jobs WHERE pipeline_id = ?", id)
		if err != nil {
			log.Fatal(err)
		}

		data := struct {
			Def      *PipelineDefinition
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
		jobHandle := r.FormValue("handle")

		var pipeline PipelineInstance
		db.Get(&pipeline, "SELECT * FROM pipelines WHERE id = ?", pipelineId)
		def := pipelineByHandle(pipeline.Handle)

		type LogLine struct {
			Stream   stdstreams.Stream
			Line     string
			LoggedAt time.Time `db:"logged_at"`
		}

		var logLines []LogLine
		db.Select(&logLines, "SELECT stream, line, logged_at FROM logs JOIN jobs ON jobs.id = logs.job_id WHERE jobs.handle = ? AND jobs.pipeline_id = ? ORDER BY logged_at ASC", jobHandle, pipelineId)

		data := struct {
			PipelineDef      *PipelineDefinition
			PipelineInstance *PipelineInstance
			JobHandle        string
			Log              []LogLine
		}{
			def,
			&pipeline,
			jobHandle,
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
