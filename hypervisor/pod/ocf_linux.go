package pod

import (
	"encoding/json"
	"github.com/opencontainers/specs"
)

func OCFConvert2Pod(ociData []byte) (*UserPod, error) {
	var s specs.LinuxSpec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, err
	}

	return OCFSpec2Pod(s.Spec, int(s.Linux.Resources.Memory.Limit>>20)), nil
}
