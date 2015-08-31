package pod

import (
	"encoding/json"
	"fmt"
	"github.com/opencontainers/specs"
	"strings"
)

func OCFSpec2Pod(s specs.Spec, memory int) *UserPod {
	container := UserContainer{
		Command:       s.Process.Args,
		Workdir:       s.Process.Cwd,
		Image:         s.Root.Path,
		RestartPolicy: "never",
	}

	for _, value := range s.Process.Env {
		fmt.Printf("env: %s\n", value)
		values := strings.Split(value, "=")
		tmp := UserEnvironmentVar{
			Env:   values[0],
			Value: values[1],
		}
		container.Envs = append(container.Envs, tmp)
	}

	userpod := &UserPod{
		Name: s.Hostname,
		Resource: UserResource{
			Memory: memory,
			Vcpu:   0,
		},
		Tty:           s.Process.Terminal,
		Type:          "OCF",
		RestartPolicy: "never",
	}

	userpod.Containers = append(userpod.Containers, container)

	userData, _ := json.Marshal(userpod)
	fmt.Printf("userData:\n%s\n", userData)

	return userpod
}
