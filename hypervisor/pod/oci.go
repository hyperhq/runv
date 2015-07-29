package pod

import (
	"github.com/opencontainers/specs"
	"encoding/json"
	"strings"
	"fmt"
)

func OCIConvert2Pod(ociData []byte) (*UserPod, error) {
	var s specs.LinuxSpec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, err
	}

	for _, cmd := range s.Process.Args {
		fmt.Printf("cmd: %s", cmd)
	}

	container := UserContainer {
		Command:	s.Process.Args,
		Workdir:	s.Process.Cwd,
		Image:		s.Root.Path,
		RestartPolicy:	"never",
	}

	for _, value := range s.Process.Env {
		fmt.Printf("evn: %s", value)
		values := strings.Split(value, "=")
		tmp := UserEnvironmentVar{
			Env:	values[0],
			Value:	values[1],
		}
		container.Envs = append(container.Envs, tmp)
	}

	userpod := &UserPod {
		Name:		s.Hostname,
		Resource:	UserResource {
			Memory:		int(s.Linux.Resources.Memory.Limit>>20),
			Vcpu:		0,
		},
		Tty:		s.Process.Terminal,
		Type:		"OCI",
		RestartPolicy:	"never",
	}

	userpod.Containers = append(userpod.Containers, container)

	userData, _:= json.Marshal(userpod)
	fmt.Printf("userData:\n%s\n", userData)

	return userpod, nil
}
