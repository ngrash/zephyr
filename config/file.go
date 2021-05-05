package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type Pipelines []Pipeline

func (ps *Pipelines) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("root element must be YAML mapping")
	}

	*ps = make([]Pipeline, len(value.Content)/2)
	for i := 0; i < len(value.Content); i += 2 {
		var res = &(*ps)[i/2]
		if err := value.Content[i].Decode(&res.Name); err != nil {
			return err
		}
		if err := value.Content[i+1].Decode(&res); err != nil {
			return err
		}
	}

	return nil
}

type Pipeline struct {
	Name     string
	Schedule string
	Jobs     Jobs
}

type Jobs []Job

func (j *Jobs) UnmarshalYAML(value *yaml.Node) error {
	*j = make([]Job, len(value.Content)/2)
	for i := 0; i < len(value.Content); i += 2 {
		var res = &(*j)[i/2]
		if err := value.Content[i].Decode(&res.Name); err != nil {
			return err
		}
		if err := value.Content[i+1].Decode(&res.Command); err != nil {
			return err
		}
	}
	return nil
}

type Job struct {
	Name    string
	Command string
}

func LoadPipelines(filename string) (Pipelines, error) {
	var ps Pipelines
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return ps, err
	}
	err = yaml.Unmarshal(bytes, &ps)
	if err != nil {
		return ps, err
	}
	return ps, nil
}
