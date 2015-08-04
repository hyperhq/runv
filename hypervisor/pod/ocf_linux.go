package pod

import (
	"github.com/opencontainers/specs"
	"encoding/json"
)

func OCFConvert2Pod(ociData []byte) (*UserPod, error) {
	var s specs.LinuxSpec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, err
	}

	return OCFSpec2Pod(s.(specs.Spec), int(int(s.Linux.Resources.Memory.Limit>>20)), nil
}
