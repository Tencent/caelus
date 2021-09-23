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
	"os"
	"testing"
	"time"
)

func TestRestoreNodeResourceCheckpoint(t *testing.T) {
	checkPointDir := "/tmp/node-checkpoint"
	checkPointKey := "node-checkpoint"
	nodeCheckPoint := &NodeResourceCheckPoint{
		ScheduleDisable: false,
		Timeseconds:     time.Now().Unix(),
	}

	nM, _ := NewNodeResourceCheckpointManager(checkPointDir, checkPointKey)

	nM.StoreNodeResourceCheckpoint(nodeCheckPoint)
	defer os.Remove(checkPointDir)

	newCheckPoint, err := nM.RestoreNodeResourceCheckpoint()
	if err != nil {
		t.Fatalf("restore node checkpoint failed: %v", err)
	}

	if newCheckPoint.ScheduleDisable != nodeCheckPoint.ScheduleDisable ||
		newCheckPoint.Timeseconds != nodeCheckPoint.Timeseconds {
		t.Fatalf("restore node checkpoint wrong, shoule be: %v, but get: %v", nodeCheckPoint, newCheckPoint)
	}
}
