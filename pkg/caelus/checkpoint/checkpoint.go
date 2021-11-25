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
)

var _ checkpointmanager.Checkpoint = &checkpointData{}

var cpm checkpointmanager.CheckpointManager

func InitCheckpointManager(checkpointDir string) error {
	var err error
	cpm, err = checkpointmanager.NewCheckpointManager(checkpointDir)
	return err
}

func Save(key string, obj interface{}) error {
	data := &checkpointData{
		Data: obj,
	}
	return cpm.CreateCheckpoint(key, data)
}

func Restore(key string, receiver interface{}) error {
	data := &checkpointData{Data: receiver}
	return cpm.GetCheckpoint(key, data)
}

type checkpointData struct {
	Data     interface{}
	Checksum checksum.Checksum
}

// MarshalCheckpoint returns marshalled checkpoing
func (r *checkpointData) MarshalCheckpoint() ([]byte, error) {
	// should set checksum the same value(zero) when calculating,
	// or will get different sum value after restoring
	r.Checksum = 0
	r.Checksum = checksum.New(r)
	return json.Marshal(*r)
}

// UnmarshalCheckpoint tries to unmarshal passed types to checkpoing
func (r *checkpointData) UnmarshalCheckpoint(blob []byte) error {
	return json.Unmarshal(blob, r)
}

// VerifyChecksum verifies that current checksum of checkpoint is valid
func (r *checkpointData) VerifyChecksum() error {
	ck := r.Checksum
	// set checksum the same value(zero) as storing value before verify
	r.Checksum = 0
	err := ck.Verify(r)
	r.Checksum = ck
	return err
}
