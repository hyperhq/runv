package pod

import (
	"fmt"
	"strings"

	"github.com/opencontainers/specs"
)

func ConvertOCF2UserContainer(s *specs.LinuxSpec, r *specs.LinuxRuntimeSpec) *UserContainer {
	container := &UserContainer{
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

func ConvertOCF2PureUserPod(s *specs.LinuxSpec, r *specs.LinuxRuntimeSpec) *UserPod {
	return &UserPod{
		Name: s.Spec.Hostname,
		Resource: UserResource{
			Memory: int(r.Linux.Resources.Memory.Limit >> 20),
			Vcpu:   0,
		},
		Tty:           s.Spec.Process.Terminal,
		Type:          "OCF",
		RestartPolicy: "never",
	}
}
