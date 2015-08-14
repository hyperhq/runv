package pod

import (
	"encoding/json"
	"github.com/opencontainers/specs"
)

func OCFConvert2Pod(ociData []byte) (*UserPod, error) {
	var s specs.Spec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, err
	}

	return OCFSpec2Pod(s, 0), nil
}
