package yarn

import (
	"github.com/tencent/caelus/pkg/caelus/checkpoint"
	"testing"
)

func TestPortCheckpoint(t *testing.T) {
	checkpoint.InitCheckpointManager("/tmp")
	defaultPort[yarnNodeManagerWebappAddress] = 12345
	storeCheckpoint()
	defaultPort[yarnNodeManagerWebappAddress] = 0
	restorePortsCheckpoint()
	if defaultPort[yarnNodeManagerWebappAddress] != 12345 {
		t.Errorf("expect restore %v to 12345, got %d", yarnNodeManagerWebappAddress, defaultPort[yarnNodeManagerWebappAddress])
	}
}
