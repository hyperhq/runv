package pod

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opencontainers/specs"
)

func ConvertOCFLinuxContainer(s specs.LinuxSpec, r specs.LinuxRuntimeSpec) UserContainer {
	container := UserContainer{
		Command:       s.Spec.Process.Args,
		Workdir:       s.Spec.Process.Cwd,
		Tty:           s.Spec.Process.Terminal,
		Image:         s.Spec.Root.Path,
		Sysctl:        r.Linux.Sysctl,
		RestartPolicy: "never",
	}

	for _, value := range s.Spec.Process.Env {
		fmt.Printf("env: %s\n", value)
		values := strings.Split(value, "=")
		tmp := UserEnvironmentVar{
			Env:   values[0],
			Value: values[1],
		}
		container.Envs = append(container.Envs, tmp)
	}

	return container
}

func ParseOCFLinuxContainerConfig(ociData []byte, runtimeData []byte) (*UserPod, *specs.RuntimeSpec, error) {
	var s specs.LinuxSpec
	var r specs.LinuxRuntimeSpec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, nil, err
	}

	if err := json.Unmarshal(runtimeData, &r); err != nil {
		return nil, nil, err
	}
	memory := int(r.Linux.Resources.Memory.Limit >> 20)

	userpod := &UserPod{
		Name: s.Spec.Hostname,
		Resource: UserResource{
			Memory: memory,
			Vcpu:   0,
		},
		Tty:           s.Spec.Process.Terminal,
		Type:          "OCF",
		RestartPolicy: "never",
	}

	userpod.Containers = append(userpod.Containers, ConvertOCFLinuxContainer(s, r))

	userData, _ := json.Marshal(userpod)
	fmt.Printf("userData:\n%s\n", userData)

	return userpod, &r.RuntimeSpec, nil
}
