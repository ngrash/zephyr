package zephyr

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/ngrash/zephyr/config"
	"github.com/ngrash/zephyr/stdstreams"
	"testing"
)

func TestRun_OneJobWithOutput(t *testing.T) {
	p := config.Pipeline{
		Name:     "Test",
		Schedule: "",
		Alert:    "",
		Jobs: config.Jobs{
			{"Test", "echo \"hello test\""},
		},
	}

	want := []string{
		"pipeline created: P0",
		"job created: 0",
		"pipeline started: P0",
		"job started: 0",
		"job (0) output to stdout: hello test",
		"job completed: 0",
		"pipeline completed: P0",
	}

	report := TestCallbacks{}

	_, wg, err := Run(p, &report)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	wg.Wait()

	if diff := cmp.Diff(want, report.Log); diff != "" {
		t.Errorf("diff +got -want:\n%s\n", diff)
	}
}

func TestRun_FailedJob(t *testing.T) {
	p := config.Pipeline{
		Name:     "Test",
		Schedule: "",
		Alert:    "",
		Jobs: config.Jobs{
			{"Test", "true"},
			{"Test", "false"},
			{"Test", "should never run"},
			{"Test", "should never run"},
		},
	}

	want := []string{
		"pipeline created: P0",
		"job created: 0",
		"job created: 1",
		"job created: 2",
		"job created: 3",
		"pipeline started: P0",
		"job started: 0",
		"job completed: 0",
		"job started: 1",
		"job failed: 1",
		"pipeline failed: P0",
		"job cancelled: 2",
		"job cancelled: 3",
	}

	report := TestCallbacks{}

	_, wg, err := Run(p, &report)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	wg.Wait()

	if diff := cmp.Diff(want, report.Log); diff != "" {
		t.Errorf("diff +got -want:\n%s\n", diff)
	}

}

type TestCallbacks struct {
	Log    []string
	nextId int
}

func (cb *TestCallbacks) CreatePipeline(config.Pipeline) PipelineInstanceId {
	id := "P0"
	cb.logf("pipeline created: %s", id)
	return PipelineInstanceId(id)
}

func (cb *TestCallbacks) StartPipeline(_ config.Pipeline, pId PipelineInstanceId) {
	cb.logf("pipeline started: %s", pId)
}

func (cb *TestCallbacks) CreateJob(config.Pipeline, PipelineInstanceId, config.Job) JobInstanceId {
	jId := JobInstanceId(cb.nextId)
	cb.nextId++
	cb.logf("job created: %d", jId)
	return jId
}

func (cb *TestCallbacks) StartJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId) {
	cb.logf("job started: %d", jId)
}

func (cb *TestCallbacks) LogJobOutput(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId, line stdstreams.Line) {
	cb.logf("job (%d) output to %s: %s", jId, line.Stream, line.Text)
}

func (cb *TestCallbacks) FailJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, id JobInstanceId) {
	cb.logf("job failed: %s", id)
}

func (cb *TestCallbacks) CompleteJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, id JobInstanceId) {
	cb.logf("job completed: %s", id)
}

func (cb *TestCallbacks) CancelJob(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, id JobInstanceId) {
	cb.logf("job cancelled: %s", id)
}

func (cb *TestCallbacks) FailPipeline(_ config.Pipeline, id PipelineInstanceId) {
	cb.logf("pipeline failed: %s", id)
}

func (cb *TestCallbacks) CompletePipeline(_ config.Pipeline, id PipelineInstanceId) {
	cb.logf("pipeline completed: %s", id)
}

func (cb *TestCallbacks) logf(format string, a ...interface{}) {
	cb.Log = append(cb.Log, fmt.Sprintf(format, a...))
}
