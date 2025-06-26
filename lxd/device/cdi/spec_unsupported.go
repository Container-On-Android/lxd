//go:build armhf || arm || arm32 || android

package cdi

import (
	"fmt"

	"tags.cncf.io/container-device-interface/specs-go"

	"github.com/canonical/lxd/lxd/instance"
	"github.com/canonical/lxd/lxd/state"
)

// Android should not be able to play with Intel and NVIDIA.
func defaultNvidiaTegraCSVFiles(rootPath string) []string {
	return []string{}
}

func generateSpec(s *state.State, cdiID ID, inst instance.Instance) (*specs.Spec, error) {
	return nil, fmt.Errorf("NVIDIA CDI operations not supported on this platform")
}
