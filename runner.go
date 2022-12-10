package zephyr

import (
	"errors"
	"fmt"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/stdstreams"
	"os/exec"
	"sync"
)

type PipelineInstanceId string
type JobInstanceId int64

func (jId JobInstanceId) String() string {
	return fmt.Sprintf("%d", jId)
}

type RunReporter interface {
	PipelineCreated(config.Pipeline) PipelineInstanceId
	PipelineStarted(config.Pipeline, PipelineInstanceId)
	JobCreated(config.Pipeline, PipelineInstanceId, config.Job) JobInstanceId
	JobStarted(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	JobOutput(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId, stdstreams.Line)
	JobFailed(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	JobCompleted(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	JobCancelled(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	PipelineFailed(config.Pipeline, PipelineInstanceId)
	PipelineCompleted(config.Pipeline, PipelineInstanceId)
}

func Run(p config.Pipeline, report RunReporter) (PipelineInstanceId, *sync.WaitGroup, error) {
	if len(p.Jobs) == 0 {
		return "", nil, errors.New("no jobs defined")
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(p.Jobs))

	// Add one for the pipeline itself
	wg.Add(1)

	pId := report.PipelineCreated(p)
	jIds := make([]JobInstanceId, len(p.Jobs))
	for i, j := range p.Jobs {
		jIds[i] = report.JobCreated(p, pId, j)
	}

	go func() {
		// One for the pipeline itself.
		defer wg.Done()

		report.PipelineStarted(p, pId)

		for idx, j := range p.Jobs {
			jId := jIds[idx]
			report.JobStarted(p, pId, j, jId)

			cmd := createCommand(p, pId, j, jId, report)

			err := cmd.Run()
			if err == nil {
				report.JobCompleted(p, pId, j, jId)
				wg.Done()
			} else {
				report.JobFailed(p, pId, j, jId)
				report.PipelineFailed(p, pId)
				wg.Done()

				for j := idx + 1; j < len(jIds); j++ {
					jId := jIds[j]
					job := p.Jobs[j]
					report.JobCancelled(p, pId, job, jId)
					wg.Done()
				}

				// We don't run any more jobs if a job failed, so we end this goroutine.
				return
			}
		}

		report.PipelineCompleted(p, pId)
	}()

	return pId, wg, nil
}

func createCommand(p config.Pipeline, pId PipelineInstanceId, j config.Job, jId JobInstanceId, report RunReporter) *exec.Cmd {
	streams := stdstreams.NewLogWithCallback(func(newLine stdstreams.Line) {
		report.JobOutput(p, pId, j, jId, newLine)
	})

	cmd := exec.Command("sh", "-c", j.Command)
	cmd.Stderr = streams.Stderr()
	cmd.Stdout = streams.Stdout()

	return cmd
}
