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

package resource

import (
	"k8s.io/klog"
	"time"

	"github.com/tencent/caelus/pkg/caelus/checkpoint"
)

const (
	timeAvailableSeconds    = 300
	disableScheduleDuration = time.Duration(5 * time.Minute)
	checkpointKey           = "node_resource"
)

// NodeResourceCheckPoint struct is used to store schedule state in a checkpoint
type NodeResourceCheckPoint struct {
	Timeseconds     int64
	ScheduleDisable bool
}

// checkScheduleDisable will check schedule state from local check point file when agent restarted,
// if the schedule is disabled, this will set schedule disable again and enable in future time.
func checkScheduleDisable(checkTime bool,
	enableSchedule, disableSchedule func() error) error {
	nodeCheckpoint := &NodeResourceCheckPoint{}
	err := checkpoint.Restore(checkpointKey, nodeCheckpoint)
	if err != nil {
		klog.Errorf("restore node resource check point err: %v", err)
		return err
	}

	if nodeCheckpoint.ScheduleDisable == true {
		klog.Warningf("found schedule state is disabled from checkpoint")
		// check if the state is too old
		if checkTime && time.Now().Unix()-nodeCheckpoint.Timeseconds > timeAvailableSeconds {
			klog.Warningf("schedule state checkpoint is too old, ignore!")
		} else {
			klog.Warningf("disable node schedule based on checkpoint")
			// disable schedule again
			disableSchedule()
			// should enable schedule if everything is ok
			klog.Warningf("enable node schedule based on checkpoint after %v", disableScheduleDuration)
			time.AfterFunc(disableScheduleDuration, func() { enableSchedule() })
		}
	}

	return nil
}

// storeCheckpoint store schedule state into checkout point file
func storeCheckpoint(scheduleState bool) error {
	nodeCheckPoint := &NodeResourceCheckPoint{
		ScheduleDisable: scheduleState,
		Timeseconds:     time.Now().Unix(),
	}

	return checkpoint.Save(checkpointKey, nodeCheckPoint)
}
