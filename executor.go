package main

import (
	"log"
	"os/exec"

	"github.com/google/uuid"
	"github.com/ngrash/zephyr/stdstreams"
)

type InstanceStatus uint8

const (
	StatusPending InstanceStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusCancelled
)

func (i InstanceStatus) String() string {
	switch i {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return "<unknown>"
	}
}

type PipelineInstance struct {
	Id     string
	Def    *PipelineDefinition
	Status InstanceStatus
	Jobs   []*JobInstance
}

type JobInstance struct {
	Def    JobDefinition
	Status InstanceStatus
	Log    *stdstreams.Log
}

type Executor struct {
	instances map[string]*PipelineInstance
}

func NewExecutor() *Executor {
	return &Executor{
		make(map[string]*PipelineInstance),
	}
}

func (e *Executor) Run(def *PipelineDefinition) string {
	log.Printf("Executor.Run({Handle: %s})", def.Handle)

	instance := &PipelineInstance{
		Id:     uuid.NewString(),
		Def:    def,
		Status: StatusPending,
		Jobs:   make([]*JobInstance, len(def.Jobs)),
	}
	for i, j := range def.Jobs {
		instance.Jobs[i] = &JobInstance{Def: j, Status: StatusPending}
	}

	e.instances[instance.Id] = instance

	go AsyncPipelineRoutine(instance)

	return instance.Id
}

func (e *Executor) PipelineInstance(id string) (*PipelineInstance, bool) {
	i, ok := e.instances[id]
	return i, ok
}

func AsyncPipelineRoutine(i *PipelineInstance) {
	i.Status = StatusRunning

	for idx, job := range i.Jobs {
		job.Status = StatusRunning
		job.Log = stdstreams.NewLog()

		cmd := exec.Command("sh", "-c", job.Def.Command)
		cmd.Stderr = job.Log.Stderr()
		cmd.Stdout = job.Log.Stdout()

		if err := cmd.Run(); err != nil {
			job.Status = StatusFailed
			i.Status = StatusFailed

			for j := idx + 1; j < len(i.Jobs); j++ {
				i.Jobs[j].Status = StatusCancelled
			}

			return
		}

		job.Status = StatusCompleted
	}

	i.Status = StatusCompleted
}
