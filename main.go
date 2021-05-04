package main

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/ngrash/zephyr/stdstreams"
)

type PipelineDefinition struct {
	Handle string
	Jobs   []JobDefinition
}

type JobDefinition struct {
	Handle  string
	Command string
}

type PipelineViewModel struct {
	PipelineDefinition
	LastRun *time.Time
	NextRun *time.Time
}

func main() {

	executor := NewExecutor()

	pipelines := []PipelineDefinition{
		{"Hello", []JobDefinition{
			{"echo", "echo Hello world"},
			{"sleep 5", "sleep 5"},
			{"ls /", "ls /"},
			{"echo2", "echo yay"},
		}},
		{"World", []JobDefinition{
			{"echo", "echo Hello world"},
			{"sleep 1", "sleep 5"},
			{"exit 1", "exit 1"},
			{"echo2", "echo yay"},
		}},
		{"Of Pipelines", []JobDefinition{}},
	}

	pipelineByHandle := func(handle string) PipelineDefinition {
		var pipeline PipelineDefinition
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
			vms[i] = PipelineViewModel{p, nil, nil}
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
		instance, _ := executor.PipelineInstance(id)
		data := struct {
			Def      PipelineDefinition
			Instance *PipelineInstance
		}{
			instance.Def,
			instance,
		}
		tmpl := template.Must(template.ParseFiles("templates/pipeline_instance.html"))
		if err := tmpl.Execute(w, data); err != nil {
			log.Print(err)
		}
	})

	http.HandleFunc("/job_instance", func(w http.ResponseWriter, r *http.Request) {
		pipelineId := r.FormValue("pipeline_id")
		jobHandle := r.FormValue("handle")

		var job *JobInstance
		instance, _ := executor.PipelineInstance(pipelineId)
		for _, jobInstance := range instance.Jobs {
			if jobInstance.Def.Handle == jobHandle {
				job = jobInstance
				break
			}
		}

		var logLines []stdstreams.Line
		if job.Log != nil {
			logLines = job.Log.Lines()
		}

		data := struct {
			PipelineDef      PipelineDefinition
			PipelineInstance *PipelineInstance
			Def              JobDefinition
			Instance         *JobInstance
			Log              []stdstreams.Line
		}{
			instance.Def,
			instance,
			job.Def,
			job,
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
