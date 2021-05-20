package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jmoiron/sqlx"
	"github.com/ngrash/zephyr/config"

	_ "github.com/mattn/go-sqlite3"
)

var scheduled map[string]*gocron.Job

var (
	smtpHost     = flag.String("smtp-host", "", "Host of SMTP server")
	smtpPort     = flag.String("smtp-port", "25", "Port of SMTP server")
	smtpUser     = flag.String("smtp-user", "", "User for authentication with SMTP server")
	smtpPassword = flag.String("smtp-password", "", "Password for authentication with SMTP server")

	baseURL = flag.String("base-url", "http://localhost:8080", "Base URL for links to this Zephyr instance")
)

func main() {
	flag.Parse()

	mailer := NewMailer(*smtpHost, *smtpPort, *smtpUser, *smtpPassword, *baseURL)

	db, err := sqlx.Connect("sqlite3", "zephyr.db?foreign_keys=on")
	if err != nil {
		log.Fatal(err)
	}
	MigrateSchema(db)

	executor := NewExecutor(db, mailer)

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
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))
	http.HandleFunc("/", indexHandler(pipelines, db))
	http.HandleFunc("/run", runHandler(pipelines, executor))
	http.HandleFunc("/pipeline_instance", pipelineHandler(pipelines, db))
	http.HandleFunc("/job_instance", jobHandler(db))

	log.Print("serving")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
