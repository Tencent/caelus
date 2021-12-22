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

package nodestore

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/net"
)

var nodeStatExample = &NodeResourceState{
	CPU:    &NodeCpu{},
	Load:   &NodeLoad{},
	Memory: &NodeMemory{},
	DiskIO: &NodeDiskIO{
		IOState: map[string]DiskIOState{
			"example": {},
		},
	},
	NetIO: &NodeNetwork{
		IfaceStats: map[string]*IfaceStat{
			"example": {},
		},
	},
	Process: &NodeProcess{},
}

// NodeResourceState describe node resource
type NodeResourceState struct {
	Timestamp time.Time    `structs:"timestamp,omitempty"`
	CPU       *NodeCpu     `structs:"cpu"`
	Load      *NodeLoad    `structs:"load"`
	Memory    *NodeMemory  `structs:"memory"`
	DiskIO    *NodeDiskIO  `structs:"diskio"`
	NetIO     *NodeNetwork `structs:"netio"`
	Process   *NodeProcess `structs:"process"`
}

// GetValue get node resource value
func (nrs *NodeResourceState) GetValue(resource, device string) (float64, error) {
	var handler func(resource, device string) (float64, error)

	prefixIndex := strings.Index(resource, "_")
	if prefixIndex == -1 {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, nrs.GetTags())
	}
	switch resource[:prefixIndex] {
	case "cpu":
		if nrs.CPU != nil {
			handler = nrs.CPU.GetValue
		}
	case "load":
		if nrs.Load != nil {
			handler = nrs.Load.GetValue
		}
	case "memory":
		if nrs.Memory != nil {
			handler = nrs.Memory.GetValue
		}
	case "diskio":
		if nrs.DiskIO != nil {
			handler = nrs.DiskIO.GetValue
		}
	case "netio":
		if nrs.NetIO != nil {
			handler = nrs.NetIO.GetValue
		}
	case "process":
		if nrs.Process != nil {
			handler = nrs.Process.GetValue
		}
	default:
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, nrs.GetTags())
	}

	if handler == nil {
		return 0, fmt.Errorf("struct has nil data")
	}
	return handler(resource, device)
}

// GetTags get all node resource tags
func (nrs *NodeResourceState) GetTags() []string {
	var tags []string
	if nrs.CPU != nil {
		tags = append(tags, nrs.CPU.GetTags()...)
	}
	if nrs.Load != nil {
		tags = append(tags, nrs.Load.GetTags()...)
	}
	if nrs.Memory != nil {
		tags = append(tags, nrs.Memory.GetTags()...)
	}
	if nrs.DiskIO != nil {
		for _, v := range nrs.DiskIO.IOState {
			tags = append(tags, v.GetTags()...)
			break
		}
	}
	if nrs.NetIO != nil {
		for _, v := range nrs.NetIO.IfaceStats {
			tags = append(tags, v.GetTags()...)
			break
		}
	}
	if nrs.Process != nil {
		tags = append(tags, nrs.Process.GetTags()...)
	}

	return tags
}

// NodeCpu record cpu state for node
type NodeCpu struct {
	// just for compute core usage per second
	state        []cpu.TimesStat `structs:"state,omitempty"`
	timestamp    time.Time       `structs:"timestamp,omitempty"`
	offlineUsage uint64          `structs:"offline_usage,omitempty"`

	// core used per second
	CpuTotal   float64   `structs:"cpu_total"`
	CpuPerCore []float64 `structs:"cpu_per_core"`
	CpuAvg     float64   `structs:"cpu_avg"`

	CpuStealTotal   float64   `structs:"cpu_steal_total"`
	CpuStealPerCore []float64 `structs:"cpu_steal_per_core"`
	CpuOfflineTotal float64   `structs:"cpu_offline_total"`
}

// GetValue get node cpu resource value
func (c *NodeCpu) GetValue(resource, device string) (float64, error) {
	valueMap := structs.Map(c)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, c.GetTags())
	}

	if len(device) == 0 {
		return value.(float64), nil
	} else {
		index, err := strconv.Atoi(device)
		if err != nil {
			return 0, fmt.Errorf("invalid cpu number(%s): %v", device, err)
		}
		if index < 0 || index >= len(c.CpuPerCore) {
			return 0, fmt.Errorf("invalid cpu number: %s", device)
		}
		return value.([]float64)[index], nil
	}
}

// GetTags get all node cpu resource tags
func (c *NodeCpu) GetTags() []string {
	var tags []string
	valueMap := structs.Map(c)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}

// NodeLoad record load state for node
type NodeLoad struct {
	Load1Min  float64 `structs:"load_1min"`
	Load5Min  float64 `structs:"load_5min"`
	Load15Min float64 `structs:"load_15min"`
}

// GetValue get node cpu load resource value
func (l *NodeLoad) GetValue(resource, device string) (float64, error) {
	valueMap := structs.Map(l)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, l.GetTags())
	}

	return value.(float64), nil
}

// GetTags get all node cpu load resource tags
func (l *NodeLoad) GetTags() []string {
	var tags []string
	valueMap := structs.Map(l)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}

// NodeMemory record memory state for node
type NodeMemory struct {
	Total      float64 `structs:"memory_total"`
	UsageTotal float64 `structs:"memory_usage_total"`
	UsageCache float64 `structs:"memory_usage_cache"`
	UsageRss   float64 `structs:"memory_usage_rss"`
	Available  float64 `structs:"memory_available"`

	OfflineTotal float64 `structs:"memory_offline_total"`
}

// GetValue get memory resource value
func (m *NodeMemory) GetValue(resource, device string) (float64, error) {
	valueMap := structs.Map(m)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, m.GetTags())
	}

	return value.(float64), nil
}

// GetTags get memory resource tags
func (m *NodeMemory) GetTags() []string {
	var tags []string
	valueMap := structs.Map(m)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}

// NodeDiskIO record disk io state for node
type NodeDiskIO struct {
	state map[string]disk.IOCountersStat `structs:"state,omitempty"`

	// disk io
	IOState map[string]DiskIOState `structs:"diskio_state"`

	timestamp time.Time `structs:"timestamp,omitempty"`
}

// GetValue get disk io resource value by device name
func (nd *NodeDiskIO) GetValue(resource, device string) (float64, error) {
	state, ok := nd.IOState[device]
	if !ok {
		return 0, fmt.Errorf("device %s not found", device)
	}

	return state.GetValue(resource, device)
}

// DiskIOState record signal disk io state
type DiskIOState struct {
	DiskReadKiBps  float64 `structs:"diskio_read_kibps"`
	DiskWriteKiBps float64 `structs:"diskio_write_kibps"`
	DiskReadIOps   float64 `structs:"diskio_read_iops"`
	DiskWriteIOps  float64 `structs:"diskio_write_iops"`
	Util           float64 `structs:"diskio_util"`
}

// GetValue get disk io resource value
func (d *DiskIOState) GetValue(resource, device string) (float64, error) {
	valueMap := structs.Map(d)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, d.GetTags())
	}

	return value.(float64), nil
}

// GetTags get disk io resource tags
func (d *DiskIOState) GetTags() []string {
	var tags []string
	valueMap := structs.Map(d)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}

// NodeNetwork record network io state for node
type NodeNetwork struct {
	//key is the iface name
	rawState   map[string]*net.IOCountersStat `structs:"-"`
	IfaceStats map[string]*IfaceStat          `structs:"netio_stats"`

	timestamp time.Time `structs:"timestamp,omitempty"`
}

// GetValue get network io value by iface device
func (nn *NodeNetwork) GetValue(resource, device string) (float64, error) {
	stat, ok := nn.IfaceStats[device]
	if !ok {
		return 0, fmt.Errorf("device not found: %s", device)
	}

	return stat.GetValue(resource, device)
}

// IfaceStat record signal network io stat
type IfaceStat struct {
	Iface string `structs:"iface"`
	// Speed is the total kilobits for the iface
	Speed int `structs:"netio_speed"`
	// NetRecvkbps is the kilobits per second for ingress
	NetRecvkbps float64 `structs:"netio_recv_kbps"`
	// NetSentkbps is the kilobits per second for egress
	NetSentkbps float64 `structs:"netio_sent_kbps"`
	// NetRecvPckps is the package per second for ingress
	NetRecvPckps float64 `structs:"netio_recv_pckps"`
	// NetSendPckps is the package per second for egress
	NetSentPckps float64 `structs:"netio_sent_pckps"`
	// NetDroIn is the package dropped per second for ingress
	NetDropIn float64 `structs:"netio_drop_in"`
	// NetDropOut is the package dropped per second for egress
	NetDropOut float64 `structs:"netio_drop_out"`
	// NetFifoIn is the number of in FIFO buffer errors
	NetFifoIn float64 `structs:"netio_fifo_in"`
	// NetFifoOut is the number of out FIFO buffer errors
	NetFifoOut float64 `structs:"netio_fifo_out"`
	// NetErrin is the number of errors while receiving
	NetErrin float64 `structs:"netio_err_in"`
	// NetErrout is the number of errors while receiving
	NetErrout float64 `structs:"netio_err_out"`
}

// GetValue get network io data by iface device
func (is *IfaceStat) GetValue(resource, device string) (float64, error) {
	valueMap := structs.Map(is)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, is.GetTags())
	}

	return value.(float64), nil
}

// GetTags get network resource tags
func (is *IfaceStat) GetTags() []string {
	var tags []string
	valueMap := structs.Map(is)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}

// NodeProcess record processes status for node
type NodeProcess struct {
	// Number of tasks in uninterruptible state
	NrUninterruptible uint64 `structs:"process_nr_uninterruptible"`
}

// GetValue get process resource value
func (p *NodeProcess) GetValue(resource, device string) (float64, error) {
	valueMap := structs.Map(p)
	value, ok := valueMap[resource]
	if !ok {
		return 0, fmt.Errorf("%s not found, supporting: %v", resource, p.GetTags())
	}

	return value.(float64), nil
}

// GetTags get process resource tags
func (p *NodeProcess) GetTags() []string {
	var tags []string
	valueMap := structs.Map(p)
	for k := range valueMap {
		tags = append(tags, k)
	}

	return tags
}
