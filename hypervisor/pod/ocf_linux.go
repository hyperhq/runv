package pod

import (
	"encoding/json"
	"github.com/opencontainers/specs"
)

func OCFConvert2Pod(ociData []byte, runtimeData []byte) (*UserPod, error) {
	var s specs.LinuxSpec
	var r specs.LinuxRuntimeSpec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, err
	}

	memory := 0
	if runtimeData != nil {
		if err := json.Unmarshal(runtimeData, &r); err != nil {
			return nil, err
		}
		memory = int(r.Linux.Resources.Memory.Limit >> 20)
	}

	return OCFSpec2Pod(s.Spec, memory), nil
}
