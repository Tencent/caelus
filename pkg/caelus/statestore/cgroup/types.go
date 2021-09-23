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

package cgroupstore

import (
	"fmt"
	"time"

	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"github.com/fatih/structs"
	cadvisorapi "github.com/google/cadvisor/info/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// CgroupStoreInterface describe cgroup resource state interface
type CgroupStoreInterface interface {
	MachineInfo() (*cadvisorapi.MachineInfo, error)

	GetCgroupResourceRangeStats(podName, podNamespace string, start, end time.Time, count int) ([]*CgroupStats, error)
	GetCgroupResourceRecentState(podName, podNamespace string, updateStats bool) (*CgroupStats, error)
	GetCgroupResourceRecentStateByPath(cgPath string, updateStats bool) (*CgroupStats, error)

	ListCgroupResourceRangeStats(start, end time.Time, count int, classFilter sets.String) (
		map[string][]*CgroupStats, error)
	ListCgroupResourceRecentState(updateStats bool, classFilter sets.String) (map[string]*CgroupStats, error)
	ListAllCgroups(classFilter sets.String) (map[string]*CgroupRef, error)

	// AddExtraCgroups will restart cadvisor manager with high cost, so should try not call the function
	AddExtraCgroups(extraCgs []string) error

	GetCgroupStoreSupportedTags() []string
}

// CgroupStore describe cgroup resource state interface inherit from state store
type CgroupStore interface {
	Run(stop <-chan struct{})
	CgroupStoreInterface
}

// CgroupStats group cgroup state options
type CgroupStats struct {
	Ref                   *CgroupRef `structs:"ref,omitempty"`
	CpuUsage              float64    `structs:"cpu_usage"`
	NrCpuThrottled        float64    `structs:"nr_cpu_throttled"`
	LoadAvg               float64    `structs:"load_avg"`
	MemoryTotalUsage      float64    `structs:"memory_total_usage"`
	MemoryWorkingSetUsage float64    `structs:"memory_working_set_usage"`
	MemoryCacheUsage      float64    `structs:"memory_cache_usage"`
	TcpOOM                float64    `structs:"tcp_oom"`
	ListenOverflows       float64    `structs:"listen_overflows"`
	RxBps                 float64    `structs:"rx_bps"`
	RxPps                 float64    `structs:"rx_pps"`
	TxBps                 float64    `structs:"tx_bps"`
	TxPps                 float64    `structs:"tx_pps"`

	Timestamp time.Time `structs:"timestamp,omitempty"`
}

// GetValue get resource data by the struct tag
func (cg *CgroupStats) GetValue(resource string) (float64, error) {
	valueMap := structs.Map(cg)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, cg.GetTags())
	}

	return value.(float64), nil
}

// GetTags get all tags for the struct
func (cg *CgroupStats) GetTags() []string {
	var tags []string
	valueMap := structs.Map(cg)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}

// CgroupRef group pod infos
type CgroupRef struct {
	Name          string
	ContainerName string
	PodName       string
	PodNamespace  string
	PodUid        string
	AppClass      appclass.AppClass
}
