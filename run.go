package zephyr

import (
	"errors"
	"fmt"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/stdstreams"
	"os/exec"
	"sync"
)

type Callbacks interface {
	PipelineCallbacks
	JobCallbacks
}

type PipelineInstanceId string

type PipelineCallbacks interface {
	CreatePipeline(config.Pipeline) PipelineInstanceId
	StartPipeline(config.Pipeline, PipelineInstanceId)
	FailPipeline(config.Pipeline, PipelineInstanceId)
	CompletePipeline(config.Pipeline, PipelineInstanceId)
}

type JobInstanceId int64

func (jId JobInstanceId) String() string {
	return fmt.Sprintf("%d", jId)
}

type JobCallbacks interface {
	CreateJob(config.Pipeline, PipelineInstanceId, config.Job) JobInstanceId
	StartJob(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	LogJobOutput(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId, stdstreams.Line)
	FailJob(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	CompleteJob(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
	CancelJob(config.Pipeline, PipelineInstanceId, config.Job, JobInstanceId)
}

func Run(p config.Pipeline, cb Callbacks) (PipelineInstanceId, *sync.WaitGroup, error) {
	if len(p.Jobs) == 0 {
		return "", nil, errors.New("no jobs defined")
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(p.Jobs))

	// Add one for the pipeline itself
	wg.Add(1)

	pId := cb.CreatePipeline(p)
	jIds := make([]JobInstanceId, len(p.Jobs))
	for i, j := range p.Jobs {
		jIds[i] = cb.CreateJob(p, pId, j)
	}

	go func() {
		// One for the pipeline itself.
		defer wg.Done()

		cb.StartPipeline(p, pId)

		for idx, j := range p.Jobs {
			jId := jIds[idx]
			cb.StartJob(p, pId, j, jId)

			cmd := createCommand(p, pId, j, jId, cb)

			err := cmd.Run()
			if err == nil {
				cb.CompleteJob(p, pId, j, jId)
				wg.Done()
			} else {
				cb.FailJob(p, pId, j, jId)
				cb.FailPipeline(p, pId)
				wg.Done()

				for j := idx + 1; j < len(jIds); j++ {
					jId := jIds[j]
					job := p.Jobs[j]
					cb.CancelJob(p, pId, job, jId)
					wg.Done()
				}

				// We don't run any more jobs if a job failed, so we end this goroutine.
				return
			}
		}

		cb.CompletePipeline(p, pId)
	}()

	return pId, wg, nil
}

func createCommand(p config.Pipeline, pId PipelineInstanceId, j config.Job, jId JobInstanceId, report Callbacks) *exec.Cmd {
	streams := stdstreams.NewLogWithCallback(func(newLine stdstreams.Line) {
		report.LogJobOutput(p, pId, j, jId, newLine)
	})

	cmd := exec.Command("sh", "-c", j.Command)
	cmd.Stderr = streams.Stderr()
	cmd.Stdout = streams.Stdout()

	return cmd
}
