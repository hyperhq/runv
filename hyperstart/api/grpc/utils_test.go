package grpc

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
)

const ociConfigFile = "config.json"

func assertIsEqual(t *testing.T, ociSpec *specs.Spec, grpcSpec *Spec) {
	assert := assert.New(t)

	// Version check
	assert.Equal(grpcSpec.Version, ociSpec.Version)

	// Process checks: User
	assert.Equal(grpcSpec.Process.User.UID, ociSpec.Process.User.UID)
	assert.Equal(grpcSpec.Process.User.GID, ociSpec.Process.User.GID)

	// Process checks: Capabilities
	assert.Equal(grpcSpec.Process.Capabilities.Bounding, ociSpec.Process.Capabilities.Bounding)
	assert.Equal(grpcSpec.Process.Capabilities.Effective, ociSpec.Process.Capabilities.Effective)
	assert.Equal(grpcSpec.Process.Capabilities.Inheritable, ociSpec.Process.Capabilities.Inheritable)
	assert.Equal(grpcSpec.Process.Capabilities.Permitted, ociSpec.Process.Capabilities.Permitted)
	assert.Equal(grpcSpec.Process.Capabilities.Ambient, ociSpec.Process.Capabilities.Ambient)

	// Annotations checks: Annotations
	assert.Equal(len(grpcSpec.Annotations), len(ociSpec.Annotations))

	for k := range grpcSpec.Annotations {
		assert.Equal(grpcSpec.Annotations[k], ociSpec.Annotations[k])
	}

	// Linux checks: Devices
	assert.Equal(len(grpcSpec.Linux.Resources.Devices), len(ociSpec.Linux.Resources.Devices))
	assert.Equal(len(grpcSpec.Linux.Resources.Devices), 1)
	assert.Equal(grpcSpec.Linux.Resources.Devices[0].Access, "rwm")

	// Linux checks: Namespaces
	assert.Equal(len(grpcSpec.Linux.Namespaces), len(ociSpec.Linux.Namespaces))
	assert.Equal(len(grpcSpec.Linux.Namespaces), 5)

	for i := range grpcSpec.Linux.Namespaces {
		assert.Equal(grpcSpec.Linux.Namespaces[i].Type, (string)(ociSpec.Linux.Namespaces[i].Type))
		assert.Equal(grpcSpec.Linux.Namespaces[i].Path, (string)(ociSpec.Linux.Namespaces[i].Path))
	}
}

func TestOCItoGRPC(t *testing.T) {
	assert := assert.New(t)
	var ociSpec specs.Spec

	configJsonBytes, err := ioutil.ReadFile(ociConfigFile)
	assert.NoError(err, "Could not open OCI config file")

	err = json.Unmarshal(configJsonBytes, &ociSpec)
	assert.NoError(err, "Could not unmarshall OCI config file")

	spec, err := OCItoGRPC(&ociSpec)
	assert.NoError(err, "Could not convert OCI config file")
	assertIsEqual(t, &ociSpec, spec)
}

func TestGRPCtoOCI(t *testing.T) {
	assert := assert.New(t)

	var ociSpec specs.Spec

	configJsonBytes, err := ioutil.ReadFile(ociConfigFile)
	assert.NoError(err, "Could not open OCI config file")

	err = json.Unmarshal(configJsonBytes, &ociSpec)
	assert.NoError(err, "Could not unmarshall OCI config file")

	grpcSpec, err := OCItoGRPC(&ociSpec)
	assert.NoError(err, "Could not convert OCI config file")

	newOciSpec, err := GRPCtoOCI(grpcSpec)
	assert.NoError(err, "Could not convert gRPC structure")

	assertIsEqual(t, newOciSpec, grpcSpec)
}
