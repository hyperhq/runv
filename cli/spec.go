package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func LoadSpec(ocffile string) (*specs.Spec, error) {
	var spec specs.Spec

	if _, err := os.Stat(ocffile); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%q doesn't exists", ocffile)
		}
		return nil, fmt.Errorf("Stat %q error: %v", ocffile, err)
	}

	ocfData, err := ioutil.ReadFile(ocffile)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(ocfData, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// LoadProcessConfig loads the process configuration from the provided path.
func LoadProcessConfig(path string) (*specs.Process, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("JSON configuration file for %s not found", path)
		}
		return nil, err
	}
	defer f.Close()
	var s *specs.Process
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}

type State struct {
	specs.State
	ShimCreateTime      uint64 `json:"shim_create_time"`
	ContainerCreateTime int64  `json:"container_create_time"`
}

func SaveStateFile(root, container string, state *State) error {
	stateFile := filepath.Join(root, container, StateJSON)
	stateData, err := json.MarshalIndent(state, "", "\t")
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}
	err = ioutil.WriteFile(stateFile, stateData, 0644)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}
	return nil
}

func LoadStateFile(root, container string) (*State, error) {
	stateFile := filepath.Join(root, container, StateJSON)
	file, err := os.Open(stateFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var state State
	if err = json.NewDecoder(file).Decode(&state); err != nil {
		return nil, fmt.Errorf("Decode state file %s error: %s", stateFile, err.Error())
	}
	return &state, nil
}

func LoadStateAndSpec(root, container string) (*State, *specs.Spec, error) {
	state, err := LoadStateFile(root, container)
	if err != nil {
		return nil, nil, err
	}
	spec, err := LoadSpec(filepath.Join(state.Bundle, SpecConfig))
	if err != nil {
		return nil, nil, err
	}
	return state, spec, nil
}
