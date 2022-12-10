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

	report := TestReporter{}

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

	report := TestReporter{}

	_, wg, err := Run(p, &report)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	wg.Wait()

	if diff := cmp.Diff(want, report.Log); diff != "" {
		t.Errorf("diff +got -want:\n%s\n", diff)
	}

}

type TestReporter struct {
	Log    []string
	nextId int
}

func (t *TestReporter) PipelineCreated(config.Pipeline) PipelineInstanceId {
	id := "P0"
	t.logf("pipeline created: %s", id)
	return PipelineInstanceId(id)
}

func (t *TestReporter) PipelineStarted(_ config.Pipeline, pId PipelineInstanceId) {
	t.logf("pipeline started: %s", pId)
}

func (t *TestReporter) JobCreated(config.Pipeline, PipelineInstanceId, config.Job) JobInstanceId {
	jId := JobInstanceId(t.nextId)
	t.nextId++
	t.logf("job created: %d", jId)
	return jId
}

func (t *TestReporter) JobStarted(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId) {
	t.logf("job started: %d", jId)
}

func (t *TestReporter) JobOutput(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, jId JobInstanceId, line stdstreams.Line) {
	t.logf("job (%d) output to %s: %s", jId, line.Stream, line.Text)
}

func (t *TestReporter) JobFailed(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, id JobInstanceId) {
	t.logf("job failed: %s", id)
}

func (t *TestReporter) JobCompleted(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, id JobInstanceId) {
	t.logf("job completed: %s", id)
}

func (t *TestReporter) JobCancelled(_ config.Pipeline, _ PipelineInstanceId, _ config.Job, id JobInstanceId) {
	t.logf("job cancelled: %s", id)
}

func (t *TestReporter) PipelineFailed(_ config.Pipeline, id PipelineInstanceId) {
	t.logf("pipeline failed: %s", id)
}

func (t *TestReporter) PipelineCompleted(_ config.Pipeline, id PipelineInstanceId) {
	t.logf("pipeline completed: %s", id)
}

func (t *TestReporter) logf(format string, a ...interface{}) {
	t.Log = append(t.Log, fmt.Sprintf(format, a...))
}
