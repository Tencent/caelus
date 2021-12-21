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

package metrics

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/tencent/caelus/pkg/caelus/diskquota"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/node"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/perf"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/rdt"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/sets"
)

// metricValue describes the metric options
type metricValue struct {
	value     float64
	labels    []string
	timestamp time.Time
}

type metricValues []metricValue

// NodeResourceMetric describe the node metric options
type NodeResourceMetric struct {
	Name        string
	Description string
	extraLabels []string
	ValueFn     func(nodeSt *nodestore.NodeResourceState) metricValues
	ValueFn2    func(stStore statestore.StateStore) metricValues
}

// the function implements prometheus interface
func (n *NodeResourceMetric) desc() *prometheus.Desc {
	return prometheus.NewDesc(n.Name, n.Description, append([]string{"node"}, n.extraLabels...), nil)
}

// ContainerResourceMetric show metrics for container
type ContainerResourceMetric struct {
	Name        string
	Description string
	extraLabels []string
	ValueFn     func(cgroupStats *cgroupstore.CgroupStats) metricValues
}

// the function implements prometheus interface
func (c *ContainerResourceMetric) desc() *prometheus.Desc {
	return prometheus.NewDesc(c.Name, c.Description,
		append([]string{"container", "pod", "namespace", "node", "appclass"}, c.extraLabels...), nil)
}

// PerfResourceMetric show metrics for perf
type PerfResourceMetric struct {
	Name        string
	Description string
	extraLabels []string
	ValueFn     func(perfMetrics *perfstore.PerfMetrics) metricValues
}

// the function implements prometheus interface
func (p *PerfResourceMetric) desc() *prometheus.Desc {
	return prometheus.NewDesc(p.Name, p.Description,
		append([]string{"container", "pod", "namespace"}, p.extraLabels...), nil)
}

// RdtResourceMetric show metrics for RDT
type RdtResourceMetric struct {
	Name        string
	Description string
	extraLabels []string
	ValueFn     func(rdtMetrics *rdtstore.RdtMetrics) metricValues
}

// the function implements prometheus interface
func (r *RdtResourceMetric) desc() *prometheus.Desc {
	return prometheus.NewDesc(r.Name, r.Description,
		append([]string{"container", "pod", "namespace"}, r.extraLabels...), nil)
}

// DiskQuotaMetric show disk quota metrics for pod
type DiskQuotaMetric struct {
	Name        string
	Description string
	extraLabels []string
	ValueFn     func(volumes *diskquota.PodVolumes) metricValues
}

// the function implements prometheus interface
func (d *DiskQuotaMetric) desc() *prometheus.Desc {
	return prometheus.NewDesc(d.Name, d.Description,
		append([]string{"pod", "namespace", "node", "appclass"}, d.extraLabels...), nil)
}

// ResourceMetrics group all metrics
type ResourceMetrics struct {
	NodeMetrics      []NodeResourceMetric
	ContainerMetrics []ContainerResourceMetric
	PerfMetrics      []PerfResourceMetric
	RdtMetrics       []RdtResourceMetric
	DiskQuotaMetrics []DiskQuotaMetric
}

// nodeCpuUsage return cpu usage for node
func nodeCpuUsage(nodeSt *nodestore.NodeResourceState) metricValues {
	res := nodeSt.CPU
	if res == nil {
		return metricValues{}
	}

	values := metricValues{
		{
			value:     res.CpuTotal,
			labels:    []string{"total"},
			timestamp: nodeSt.Timestamp,
		},
	}
	// show signal cpu usage for each core
	for i, v := range res.CpuPerCore {
		values = append(values, metricValue{
			value:     v,
			labels:    []string{fmt.Sprintf("cpu%02d", i)},
			timestamp: nodeSt.Timestamp,
		})
	}
	return values
}

// nodeCpuSteal return steal cpu for node
func nodeCpuSteal(nodeSt *nodestore.NodeResourceState) metricValues {
	res := nodeSt.CPU
	if res == nil {
		return metricValues{}
	}

	values := metricValues{
		{
			value:     res.CpuStealTotal,
			labels:    []string{"total"},
			timestamp: nodeSt.Timestamp,
		},
	}
	// show signal cpu steal time for each core
	for i, v := range res.CpuStealPerCore {
		values = append(values, metricValue{
			value:     v,
			labels:    []string{fmt.Sprintf("cpu%02d", i)},
			timestamp: nodeSt.Timestamp,
		})
	}
	return values
}

// nodeLoadUsage return load value for node
func nodeLoadUsage(nodeSt *nodestore.NodeResourceState) metricValues {
	res := nodeSt.Load
	if res == nil {
		return metricValues{}
	}

	return metricValues{
		{
			value:     res.Load1Min,
			timestamp: nodeSt.Timestamp,
			labels:    []string{"1min"},
		},
		{
			value:     res.Load5Min,
			timestamp: nodeSt.Timestamp,
			labels:    []string{"5min"},
		},
		{
			value:     res.Load15Min,
			timestamp: nodeSt.Timestamp,
			labels:    []string{"15min"},
		},
	}
}

// nodeMemoryUsage return memory usage for node
func nodeMemoryUsage(nodeSt *nodestore.NodeResourceState) metricValues {
	res := nodeSt.Memory
	if res == nil {
		return metricValues{}
	}

	// distinguish cache and rss usage, the cache could be reclaimed in emergency
	return metricValues{
		{
			value:     bytesToGi(res.UsageTotal),
			timestamp: nodeSt.Timestamp,
			labels:    []string{"total"},
		},
		{
			value:     bytesToGi(res.UsageRss),
			timestamp: nodeSt.Timestamp,
			labels:    []string{"rss"},
		},
		{
			value:     bytesToGi(res.UsageCache),
			timestamp: nodeSt.Timestamp,
			labels:    []string{"cache"},
		},
	}
}

// nodeDiskIOUsage return disk io usage for node
func nodeDiskIOUsage(nodeSt *nodestore.NodeResourceState) metricValues {
	values := metricValues{}

	res := nodeSt.DiskIO
	if res == nil {
		return values
	}
	// distinguish with block devices
	for d, v := range res.IOState {
		values = append(values, metricValue{
			value:     v.DiskWriteKiBps,
			labels:    []string{d, "writeKiBps"},
			timestamp: nodeSt.Timestamp,
		})
		values = append(values, metricValue{
			value:     v.DiskReadKiBps,
			labels:    []string{d, "readKiBps"},
			timestamp: nodeSt.Timestamp,
		})
		values = append(values, metricValue{
			value:     v.DiskWriteIOps,
			labels:    []string{d, "writeIOps"},
			timestamp: nodeSt.Timestamp,
		})
		values = append(values, metricValue{
			value:     v.DiskReadIOps,
			labels:    []string{d, "readIOps"},
			timestamp: nodeSt.Timestamp,
		})
		values = append(values, metricValue{
			value:     v.Util,
			labels:    []string{d, "util"},
			timestamp: nodeSt.Timestamp,
		})
	}
	return values
}

// nodeNetworkUsage return network usage for node, including all kinds of unexpect state
func nodeNetworkUsage(nodeSt *nodestore.NodeResourceState) metricValues {
	values := metricValues{}

	res := nodeSt.NetIO
	if res == nil {
		return values
	}
	// distinguish with network ifaces
	for iface, stat := range res.IfaceStats {
		values = append(values, []metricValue{
			{
				value:     stat.NetRecvkbps,
				labels:    []string{iface, "recvkbps"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetSentkbps,
				labels:    []string{iface, "sentkbps"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetRecvPckps,
				labels:    []string{iface, "recvPckps"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetSentPckps,
				labels:    []string{iface, "sentPckps"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetDropOut,
				labels:    []string{iface, "dropOut"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetDropIn,
				labels:    []string{iface, "dropIn"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetFifoIn,
				labels:    []string{iface, "fifoIn"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetFifoOut,
				labels:    []string{iface, "fifoOut"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetErrin,
				labels:    []string{iface, "errin"},
				timestamp: nodeSt.Timestamp,
			},
			{
				value:     stat.NetErrout,
				labels:    []string{iface, "errout"},
				timestamp: nodeSt.Timestamp,
			},
		}...)
	}
	return values
}

// nodeProcessMetrics return process status for node
func nodeProcessMetrics(nodeSt *nodestore.NodeResourceState) metricValues {
	res := nodeSt.Process
	if res == nil {
		return metricValues{}
	}

	// just show processes in D state
	values := metricValues{
		{
			value:     float64(res.NrUninterruptible),
			labels:    []string{"nrUninterruptible"},
			timestamp: nodeSt.Timestamp,
		},
	}
	return values
}

// containerCpuUsage return cpu usage for container
func containerCpuUsage(cgroupStats *cgroupstore.CgroupStats) metricValues {
	return metricValues{
		{
			value:     cgroupStats.CpuUsage,
			timestamp: cgroupStats.Timestamp,
		},
	}
}

// containerMemoryUsage return memory usage for container
func containerMemoryUsage(cgroupStats *cgroupstore.CgroupStats) metricValues {
	return metricValues{
		{
			value:     bytesToGi(cgroupStats.MemoryTotalUsage),
			timestamp: cgroupStats.Timestamp,
			labels:    []string{"total"},
		},
		{
			value:     bytesToGi(cgroupStats.MemoryWorkingSetUsage),
			timestamp: cgroupStats.Timestamp,
			labels:    []string{"rss"},
		},
		{
			value:     bytesToGi(cgroupStats.MemoryCacheUsage),
			timestamp: cgroupStats.Timestamp,
			labels:    []string{"cache"},
		},
	}
}

// containerTcpErrors show kinds of TCP error
func containerTcpErrors(cgroupStats *cgroupstore.CgroupStats) metricValues {
	return metricValues{
		{
			value:     cgroupStats.TcpOOM,
			labels:    []string{"TCPAbortOnMemory"},
			timestamp: cgroupStats.Timestamp,
		},
		{
			value:     cgroupStats.ListenOverflows,
			labels:    []string{"ListenOverflows"},
			timestamp: cgroupStats.Timestamp,
		},
	}
}

// nodeAppclassUsage return cpu usage percent of online and offline jobs
func nodeAppclassUsage(stStore statestore.StateStore) metricValues {
	cgroupStats, err := stStore.ListCgroupResourceRecentState(false,
		sets.NewString(appclass.AppClassOnline, appclass.AppClassSystem, appclass.AppClassOffline))
	if err != nil || len(cgroupStats) == 0 {
		return metricValues{}
	}

	var (
		offlineCpu, onlineCpu                                        float64
		offlineMemTotal, offlineMemRSS, onlineMemTotal, onlineMemRSS float64
		timestamp                                                    time.Time
	)
	for _, stat := range cgroupStats {
		timestamp = stat.Timestamp
		if stat.Ref.AppClass == appclass.AppClassOnline ||
			stat.Ref.AppClass == appclass.AppClassSystem {
			// online resource including system module resource usage
			onlineCpu += stat.CpuUsage
			onlineMemTotal += stat.MemoryTotalUsage
			onlineMemRSS += stat.MemoryWorkingSetUsage
			continue
		} else if stat.Ref.AppClass == appclass.AppClassOffline {
			// offline pod usage
			offlineCpu += stat.CpuUsage
			offlineMemTotal += stat.MemoryTotalUsage
			offlineMemRSS += stat.MemoryWorkingSetUsage
		}
	}
	return append(metricValues{
		{
			value:     onlineCpu,
			timestamp: timestamp,
			labels:    []string{"cpu", "online", "false"},
		},
		{
			value:     offlineCpu,
			timestamp: timestamp,
			labels:    []string{"cpu", "offline", "false"},
		},
		{
			value:     bytesToGi(onlineMemTotal),
			timestamp: timestamp,
			labels:    []string{"mem", "online", "false"},
		},
		{
			value:     bytesToGi(onlineMemRSS),
			timestamp: timestamp,
			labels:    []string{"mem_rss", "online", "false"},
		},
		{
			value:     bytesToGi(offlineMemTotal),
			timestamp: timestamp,
			labels:    []string{"mem", "offline", "false"},
		},
		{
			value:     bytesToGi(offlineMemRSS),
			timestamp: timestamp,
			labels:    []string{"mem_rss", "offline", "false"},
		},
	}, percentUsageMetrics(stStore, offlineCpu, onlineCpu, offlineMemTotal, offlineMemRSS,
		onlineMemTotal, onlineMemRSS, timestamp)...)
}

// percentUsageMetrics show resource usage in percent
func percentUsageMetrics(stStore statestore.StateStore, offlineCpu, onlineCpu, offlineMemTotal,
	offlineMemRSS, onlineMemTotal, onlineMemRSS float64, timestamp time.Time) metricValues {
	machine, err := stStore.MachineInfo()
	if err != nil {
		return metricValues{}
	}
	cpus := float64(machine.NumCores)
	mem := float64(machine.MemoryCapacity)
	return metricValues{
		{
			value:     randThree(onlineCpu / cpus),
			timestamp: timestamp,
			labels:    []string{"cpu", "online", "true"},
		},
		{
			value:     randThree(offlineCpu / cpus),
			timestamp: timestamp,
			labels:    []string{"cpu", "offline", "true"},
		},
		{
			value:     randThree(onlineMemTotal / mem),
			timestamp: timestamp,
			labels:    []string{"mem", "online", "true"},
		},
		{
			value:     randThree(onlineMemRSS / mem),
			timestamp: timestamp,
			labels:    []string{"mem_rss", "online", "true"},
		},
		{
			value:     randThree(offlineMemTotal / mem),
			timestamp: timestamp,
			labels:    []string{"mem", "offline", "true"},
		},
		{
			value:     randThree(offlineMemRSS / mem),
			timestamp: timestamp,
			labels:    []string{"mem_rss", "offline", "true"},
		},
	}
}

func randThree(f float64) float64 {
	return math.Round(f*1000) / 1000
}

// perfData return perf related metrics
func perfData(perfMetrics *perfstore.PerfMetrics) metricValues {
	jobName := getJobName(perfMetrics.Spec.PodName, perfMetrics.Spec.ContainerName)
	return metricValues{
		{
			value:     perfMetrics.Value.Cycles,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"cycles", jobName},
		},
		{
			value:     perfMetrics.Value.Instructions,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"instructions", jobName},
		},
		{
			value:     perfMetrics.Value.CPI,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"cpi", jobName},
		},
		{
			value:     perfMetrics.Value.CPUUsage,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"cpu_usage", jobName},
		},
		{
			value:     perfMetrics.Value.CacheMisses,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"cache-misses", jobName},
		},
		{
			value:     perfMetrics.Value.CacheReference,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"cache-reference", jobName},
		},
		{
			value:     perfMetrics.Value.LLCOccupancy,
			timestamp: perfMetrics.Value.Timestamp,
			labels:    []string{"llc-occupancy", jobName},
		},
	}
}

// rdtData return RDT related metrics, including
// - instructions per cycle
// - cache misses
// - last level cache
// - local memory bandwidth
// - remote memory bandwidth
func rdtData(rdtMetrics *rdtstore.RdtMetrics) metricValues {
	return metricValues{
		{
			value:     rdtMetrics.Value.IPC,
			timestamp: rdtMetrics.Value.Timestamp,
			labels:    []string{rdtMetrics.Value.Cores, "ipc"},
		},
		{
			value:     rdtMetrics.Value.Missees,
			timestamp: rdtMetrics.Value.Timestamp,
			labels:    []string{rdtMetrics.Value.Cores, "cache-misses"},
		},
		{
			value:     rdtMetrics.Value.LLC,
			timestamp: rdtMetrics.Value.Timestamp,
			labels:    []string{rdtMetrics.Value.Cores, "llc"},
		},
		{
			value:     rdtMetrics.Value.MBL,
			timestamp: rdtMetrics.Value.Timestamp,
			labels:    []string{rdtMetrics.Value.Cores, "mbl"},
		},
		{
			value:     rdtMetrics.Value.MBR,
			timestamp: rdtMetrics.Value.Timestamp,
			labels:    []string{rdtMetrics.Value.Cores, "mbr"},
		},
	}
}

// diskQuotaSize return volume request quota
func diskQuotaRequest(podVolumes *diskquota.PodVolumes) metricValues {
	var metrics []metricValue
	for vType, volumes := range podVolumes.Volumes {
		for name, path := range volumes.Paths {
			if path != nil && path.Size != nil {
				size := path.Size
				mode := "exclusive"
				if path.SharedInfo != nil {
					mode = "shared"
				}
				if size.Quota != 0 {
					metrics = append(metrics, metricValue{
						value:     bytesToGi(float64(size.Quota)),
						timestamp: time.Now(),
						labels:    []string{vType.String(), name, "quota", mode},
					})
				}
				// support inodes metric
				if size.Inodes != 0 {
					metrics = append(metrics, metricValue{
						value:     float64(size.Inodes),
						timestamp: time.Now(),
						labels:    []string{vType.String(), name, "inodes", mode},
					})
				}
			}
		}
	}
	return metrics
}

// diskQuotaUsed return volume used quota, distinguish with exclusive or shared mode
func diskQuotaUsed(podVolumes *diskquota.PodVolumes) metricValues {
	var metrics []metricValue
	for vType, volumes := range podVolumes.Volumes {
		for name, path := range volumes.Paths {
			if path != nil && path.Size != nil {
				size := path.Size
				mode := "exclusive"
				if path.SharedInfo != nil {
					mode = "shared"
				}
				metrics = append(metrics, metricValue{
					value:     bytesToGi(float64(size.QuotaUsed)),
					timestamp: time.Now(),
					labels:    []string{vType.String(), name, "quota", mode},
				})
				// output inodes usage
				metrics = append(metrics, metricValue{
					value:     float64(size.InodesUsed),
					timestamp: time.Now(),
					labels:    []string{vType.String(), name, "inodes", mode},
				})
			}
		}
	}

	return metrics
}

// nodeLevelMetrics return all metrics for node level
func nodeLevelMetrics() []NodeResourceMetric {
	return []NodeResourceMetric{
		{
			Name:        "caelus_node_cpu_usage",
			Description: "cpu core consumed by the node per second",
			extraLabels: []string{"cpu"},
			ValueFn:     nodeCpuUsage,
		},
		{
			Name:        "caelus_node_cpu_load",
			Description: "cpu load avg",
			extraLabels: []string{"duration"},
			ValueFn:     nodeLoadUsage,
		},
		{
			Name:        "caelus_node_cpu_steal",
			Description: "cpu core steal",
			extraLabels: []string{"cpu"},
			ValueFn:     nodeCpuSteal,
		},
		{
			Name:        "caelus_node_memory_usage",
			Description: "node memory usage",
			extraLabels: []string{"kind"},
			ValueFn:     nodeMemoryUsage,
		},
		{
			Name:        "caelus_node_disk_io_usage",
			Description: "disk io consumed by the node per second",
			extraLabels: []string{"disk", "flag"},
			ValueFn:     nodeDiskIOUsage,
		},
		{
			Name:        "caelus_node_network_usage",
			Description: "network consumed by the node per second",
			extraLabels: []string{"iface", "flag"},
			ValueFn:     nodeNetworkUsage,
		},
		{
			Name:        "caelus_node_process",
			Description: "node process status",
			extraLabels: []string{"process"},
			ValueFn:     nodeProcessMetrics,
		},
		{
			Name:        "caelus_node_appclass_usage",
			Description: "node appclass usage",
			extraLabels: []string{"resource", "appclass", "percent"},
			ValueFn2:    nodeAppclassUsage,
		},
	}
}

// containerLevelMetrics return all metrics for container level
func containerLevelMetrics() []ContainerResourceMetric {
	return []ContainerResourceMetric{
		{
			Name:        "caelus_container_cpu_usage",
			Description: "container cpu core usage",
			ValueFn:     containerCpuUsage,
		},
		{
			Name:        "caelus_container_memory_usage",
			Description: "container memory usage",
			extraLabels: []string{"kind"},
			ValueFn:     containerMemoryUsage,
		},
		{
			Name:        "caelus_container_tcp_errors",
			Description: "container tcp errors",
			extraLabels: []string{"kind"},
			ValueFn:     containerTcpErrors,
		},
	}
}

// perfLevelMetrics return all perf metrics
func perfLevelMetrics() []PerfResourceMetric {
	return []PerfResourceMetric{
		{
			Name:        "caelus_container_perf_data",
			Description: "perf data for container",
			extraLabels: []string{"event", "job_name"},
			ValueFn:     perfData,
		},
	}
}

// rdtLevelMetrics return all RDT metrics
func rdtLevelMetrics() []RdtResourceMetric {
	return []RdtResourceMetric{
		{
			Name:        "caelus_container_rdt_data",
			Description: "rdt data for container",
			extraLabels: []string{"cores", "event"},
			ValueFn:     rdtData,
		},
	}
}

// diskQuotaLevelMetrics return all disk quota metrics
func diskQuotaLevelMetrics() []DiskQuotaMetric {
	return []DiskQuotaMetric{
		{
			Name:        "caelus_pod_volume_request",
			Description: "volume size request for pod",
			extraLabels: []string{"volume", "name", "type", "mode"},
			ValueFn:     diskQuotaRequest,
		},
		{
			Name:        "caelus_pod_volume_used",
			Description: "volume size used for pod",
			extraLabels: []string{"volume", "name", "type", "mode"},
			ValueFn:     diskQuotaUsed,
		},
	}
}

// ResourceMetricsConfig return all metrics
func ResourceMetricsConfig() ResourceMetrics {
	nodeMetrics := nodeLevelMetrics()
	containerMetrics := containerLevelMetrics()
	perfMetrics := perfLevelMetrics()
	rdtMetrics := rdtLevelMetrics()
	diskQuotaMetrics := diskQuotaLevelMetrics()

	return ResourceMetrics{
		NodeMetrics:      nodeMetrics,
		ContainerMetrics: containerMetrics,
		PerfMetrics:      perfMetrics,
		RdtMetrics:       rdtMetrics,
		DiskQuotaMetrics: diskQuotaMetrics,
	}
}

// bytesToGi translates bytes to Gi, and also keep three digital.
func bytesToGi(mBytes float64) float64 {
	return float64(int64(float64(int64(mBytes/1024/1024+0.5))/1024*1000)) / 1000
}

func getJobName(podName, containerName string) string {
	jobName := containerName
	if podName != "" {
		delimeterIndex := strings.LastIndex(podName, "-")
		if delimeterIndex != -1 {
			jobName = podName[:delimeterIndex]
		}
	}
	return jobName
}
