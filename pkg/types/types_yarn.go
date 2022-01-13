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

package types

import (
	"fmt"
	"time"
)

// NMCapacity show nodemanager capacity
type NMCapacity struct {
	Vcores    int64 `json:"vcores"`
	Millcores int64 `json:"-"`
	MemoryMB  int64 `json:"memory-mb"`
}

// ConfigKey describe which keys need to check from remote request
type ConfigKey struct {
	ConfigFile string   `json:"configFile,omitempty"`
	ConfigKeys []string `json:"configKeys,omitempty"`
	ConfigAll  bool     `json:"configAll,omitempty"`
}

// ConfigProperty show property operations
type ConfigProperty struct {
	ConfigFile     string            `json:"configFile,omitempty"`
	ConfigProperty map[string]string `json:"configProperty,omitempty"`
	ConfigAdd      bool              `json:"configAdd,omitempty"`
	ConfigDel      bool              `json:"configDel,omitempty"`
}

// NMStatus show nodemanager status
type NMStatus struct {
	Pid         int    `json:"pid"`
	State       string `json:"state"`
	Description string `json:"description"`
}

// NMContainerIds describe nodemanager container id list
type NMContainerIds struct {
	Cids []string `json:"cids"`
}

// NMContainersState describe nodemanager container state list
type NMContainersState struct {
	Cstats map[string]*ContainerState `json:"cstats"`
}

// ContainerState describe nodemanager container state
type ContainerState struct {
	UsedMemMB float32   `json:"usedMemMB"`
	UsedCPU   float32   `json:"usedCpu"`
	CgPath    string    `json:"cgPath"`
	PidList   []int     `json:"pidList"`
	StartTime time.Time `json:"start_time"`
}

// NMDiskPartition show all disk partitions name
type NMDiskPartition struct {
	PartitionsName []string `json:"partitionsName"`
}

// NMContainer show container info
type NMContainer struct {
	ID                  string    `json:"id"`
	State               string    `json:"state"`
	ExitCode            int       `json:"exitCode"`
	Diagnostics         string    `json:"diagnostics"`
	User                string    `json:"user"`
	TotalMemoryNeededMB int64     `json:"totalMemoryNeededMB"`
	TotalVCoresNeeded   float32   `json:"totalVCoresNeeded"`
	UsedMemoryMB        float32   `json:"usedMemoryMB"`
	UsedVCores          float32   `json:"usedVCores"`
	ContainerLogsLink   string    `json:"containerLogsLink"`
	NodeID              string    `json:"nodeId"`
	LogDirs             []string  `json:"klogDirs"`
	LocalDirs           []string  `json:"localDirs"`
	IsAM                bool      `json:"isAm"`
	AppID               string    `json:"appId"`
	Pid                 int       `json:"-"`
	StartTime           time.Time `json:"-"`
}

// String format nodemanager container struct
func (c NMContainer) String() string {
	return fmt.Sprintf("%s %s %d, isAM %t, memory %d, vcore %f, usedMem %f,"+
		"usedVcore: %f, pid %d, start %v", c.ID, c.State, c.ExitCode, c.IsAM, c.TotalMemoryNeededMB,
		c.TotalVCoresNeeded, c.UsedMemoryMB, c.UsedVCores, c.Pid, c.StartTime)
}

// NMContainers describe nodemanager container list
type NMContainers struct {
	Container []NMContainer `json:"container"`
}

// NMContainersWrapper show the container struct from nodemanager API
/*
curl http://x.x.x.x:10001/ws/v1/node/containers
{
	"containers": {
		"container":[
		{
			"id": "xx",
			"state": "xx"
		},
		{
			"id": "xx",
			"state": "xx"
		}
		]
	}
}
*/
type NMContainersWrapper struct {
	NMContainers `json:"containers"`
}
