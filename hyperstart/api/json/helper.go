package json

import (
	"strconv"
	"strings"

	ocispecs "github.com/opencontainers/runtime-spec/specs-go"
)

func userFromOci(p *Process, u *ocispecs.User) {
	if u == nil || (u.UID == 0 && u.GID == 0 && len(u.AdditionalGids) == 0) {
		return
	}

	if u.UID != 0 {
		p.User = strconv.FormatUint(uint64(u.UID), 10)
	}
	if u.GID != 0 {
		p.Group = strconv.FormatUint(uint64(u.GID), 10)
	}
	if len(u.AdditionalGids) > 0 {
		p.AdditionalGroups = []string{}
		for _, gid := range u.AdditionalGids {
			p.AdditionalGroups = append(p.AdditionalGroups, strconv.FormatUint(uint64(gid), 10))
		}
	}
}

func ProcessFromOci(processID string, process *ocispecs.Process) *Process {
	envs := []EnvironmentVar{}

	for _, v := range process.Env {
		if eqlIndex := strings.Index(v, "="); eqlIndex > 0 {
			envs = append(envs, EnvironmentVar{
				Env:   v[:eqlIndex],
				Value: v[eqlIndex+1:],
			})
		}
	}

	p := &Process{
		Id:       processID,
		Terminal: process.Terminal,
		Args:     process.Args,
		Envs:     envs,
		Workdir:  process.Cwd,
	}
	userFromOci(p, &process.User)
	return p
}
