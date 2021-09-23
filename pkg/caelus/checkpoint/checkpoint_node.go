/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 *
 * You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package checkpoint

import (
	"encoding/json"

	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/checksum"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/errors"
)

var _ checkpointmanager.Checkpoint = &NodeResourceCheckPoint{}

// NodeResourceCheckPoint struct is used to store schedule state in a checkpoint
type NodeResourceCheckPoint struct {
	Timeseconds     int64
	ScheduleDisable bool
	Checksum        checksum.Checksum
}

// NewNodeResourceCheckPoint returns an instance of CheckPoint
func NewNodeResourceCheckPoint() *NodeResourceCheckPoint {
	return &NodeResourceCheckPoint{}
}

// MarshalCheckpoint returns marshalled checkpoing
func (r *NodeResourceCheckPoint) MarshalCheckpoint() ([]byte, error) {
	// should set checksum the same value(zero) when calculating,
	// or will get different sum value after restoring
	r.Checksum = 0
	r.Checksum = checksum.New(r)
	return json.Marshal(*r)
}

// UnmarshalCheckpoint tries to unmarshal passed types to checkpoing
func (r *NodeResourceCheckPoint) UnmarshalCheckpoint(blob []byte) error {
	return json.Unmarshal(blob, r)
}

// VerifyChecksum verifies that current checksum of checkpoint is valid
func (r *NodeResourceCheckPoint) VerifyChecksum() error {
	ck := r.Checksum
	// set checksum the same value(zero) as storing value before verify
	r.Checksum = 0
	err := ck.Verify(r)
	r.Checksum = ck
	return err
}

// NodeResourceCheckpointManager struct is used to manage checkpoint
type NodeResourceCheckpointManager struct {
	checkpointName    string
	checkpointDir     string
	checkpointManager checkpointmanager.CheckpointManager
}

// NewNodeResourceCheckpointManager returns an instance of CheckPointManager
func NewNodeResourceCheckpointManager(checkpointDir, checkpointName string) (*NodeResourceCheckpointManager, error) {
	manager, err := checkpointmanager.NewCheckpointManager(checkpointDir)
	if err != nil {
		return nil, err
	}

	return &NodeResourceCheckpointManager{
		checkpointDir:     checkpointDir,
		checkpointName:    checkpointName,
		checkpointManager: manager,
	}, nil
}

// RestoreNodeResourceCheckpoint restore checkpoint
func (r *NodeResourceCheckpointManager) RestoreNodeResourceCheckpoint() (*NodeResourceCheckPoint, error) {
	checkpoint := NewNodeResourceCheckPoint()
	err := r.checkpointManager.GetCheckpoint(r.checkpointName, checkpoint)
	if err != nil {
		if err == errors.ErrCheckpointNotFound {
			return checkpoint, nil
		}
		return nil, err
	}

	return checkpoint, nil
}

// StoreNodeResourceCheckpoint store checkpoint
func (r *NodeResourceCheckpointManager) StoreNodeResourceCheckpoint(checkpoint *NodeResourceCheckPoint) error {
	return r.checkpointManager.CreateCheckpoint(r.checkpointName, checkpoint)
}
