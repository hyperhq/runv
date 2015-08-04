package pod

import (
	"github.com/opencontainers/specs"
	"encoding/json"
)

func OCIConvert2Pod(ociData []byte) (*UserPod, error) {
	var s specs.Spec

	if err := json.Unmarshal(ociData, &s); err != nil {
		return nil, err
	}

	return OCFSpec2Pod(s, 0), nil
}
