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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	global "github.com/tencent/caelus/pkg/types"
	"k8s.io/api/core/v1"
)

var (
	// NodeResourceTypePredict shows total resource of the node
	NodeResourceTypeTotal = "total"
	// NodeResourceTypeBuffer shows buffer resource between online and offline
	NodeResourceTypeBuffer = "buffer"
	// NodeResourceTypeOnlinePredict shows predicted total online resources
	NodeResourceTypeOnlinePredict = types.NodeResourceTypeOnlinePredict
	// NodeResourceTypeOfflinePredict shows: total - buffer - predicted_online
	NodeResourceTypeOfflinePredict = "offline_predict"
	// NodeResourceTypeOfflineConflict shows: total - buffer - predicted_online - conflicted
	NodeResourceTypeOfflineConflict = "offline_conflict"
	// NodeResourceTypeOfflineFormat shows: total - buffer - predicted_online - conflicted - formated(based on disks)
	NodeResourceTypeOfflineFormat = "offline_format"
	// NodeResourceTypeOfflineCapacity shows capacity resource for offline slave node
	NodeResourceTypeOfflineCapacity = "offline_capacity"
	// NodeResourceTypeOfflineAllocated shows total resource for allocated offline jobs
	NodeResourceTypeOfflineAllocated = "offline_allocated"
	// NodeResourceTypeOfflineDisks shows cpu resource decrease because of disk limit
	NodeResourceTypeOfflineDisks = "offline_disks"

	metricNameInterferenceCounter  = "interferenceCounter"
	metricNameKillCounter          = "killCounter"
	metricNameNodeResource         = "nodeResource"
	metricNameNodeScheduleDisabled = "scheduleDisabled"
	metricsNameSLONotMetCounter    = "sloNotMetCounter"
	metricsNameOnlineJob           = "onlineJob"
	metricsDiskSpace               = "diskSpace"
	metricsNameDiskSpaceLimited    = "diskSpaceLimited"

	metricsAlarmCounter = "alarmCounter"

	totalMetrics = map[string]prometheus.Collector{
		// InterferenceCounter is the interference count
		metricNameInterferenceCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "caelus_interference_counter",
			Help: "num of interferences",
		}, []string{"node", "type"}),

		// KillCounter counts the times of killing container or pod
		metricNameKillCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "caelus_kill_counter",
			Help: "counts the times of killing container or pod",
		}, []string{"node", "type"}),

		// NodeResourceMetrics counts all kinds of node resource, such as predict and capacity
		metricNameNodeResource: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "caelus_node_resource",
			Help: "caelus node resource quantity of all kinds",
		}, []string{"node", "resource", "type"}),

		// NodeScheduleDisabled counts if the node has been disable schedule
		metricNameNodeScheduleDisabled: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "caelus_node_schedule_disabled",
			Help: "caelus node offline job schedule disabled",
		}, []string{"node"}),

		// sloNotMetCounter counts the times of apps's SLO are not met
		metricsNameSLONotMetCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "caelus_slo_not_met_counter",
			Help: "counts the times of apps' SLO are not met",
		}, []string{"node", "app"}),

		// metricsNameOnlineJob records online jobs' metrics and slo value
		metricsNameOnlineJob: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "caelus_online_job",
			Help: "caelus online jobs metrics and slo value",
		}, []string{"node", "jobname", "metrics"}),
		metricsDiskSpace: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "caelus_disk_space_gb",
			Help: "caelus node disk space",
		}, []string{"node", "type", "mountpoint"}),
		// metricNameDiskSpaceLimited counts if the node has little disk space, and cannot run offline job
		metricsNameDiskSpaceLimited: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "caelus_node_disk_space_limited",
			Help: "caelus node disk space limited",
		}, []string{"node"}),

		metricsAlarmCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "caelus_alarm_counter",
			Help: "counts of alarm message",
		}, []string{"node"}),
	}
)

// RegisterTotalMetrics registers total metrics
func RegisterTotalMetrics(reg *prometheus.Registry) {
	for _, metric := range totalMetrics {
		reg.MustRegister(metric)
	}
}

// InterferenceCounterInc increases interference count number
func InterferenceCounterInc(metricsKind string) {
	nodeName := util.NodeIP()
	interferenceCounter := totalMetrics[metricNameInterferenceCounter].(*prometheus.CounterVec)
	interferenceCounter.WithLabelValues(nodeName, metricsKind).Inc()
}

// KillCounterInc increases kill count number
func KillCounterInc(offlineType string) {
	nodeName := util.NodeIP()
	interferenceCounter := totalMetrics[metricNameKillCounter].(*prometheus.CounterVec)
	interferenceCounter.WithLabelValues(nodeName, offlineType).Inc()
}

// NodeResourceMetricsReset resets node offline resource quantity, the resType should be predict or capacity
func NodeResourceMetricsReset(res v1.ResourceList, resType string) {
	nodeName := util.NodeIP()
	cpu := float64(res.Cpu().MilliValue()) / float64(types.CPUUnit)
	mem := float64(int64(float64(res.Memory().Value()/types.MemUnit)/1024*1000)) / 1000

	nodeResourceMetrics := totalMetrics[metricNameNodeResource].(*prometheus.GaugeVec)
	nodeResourceMetrics.With(prometheus.Labels{"node": nodeName, "resource": "cpu", "type": resType}).Set(cpu)
	nodeResourceMetrics.With(prometheus.Labels{"node": nodeName, "resource": "memory", "type": resType}).Set(mem)
}

// NodeScheduleDisabled records nodes number in schedule disabled state
func NodeScheduleDisabled(disabled float64) {
	nodeName := util.NodeIP()
	scheduleDisabledMetrics := totalMetrics[metricNameNodeScheduleDisabled].(*prometheus.GaugeVec)
	scheduleDisabledMetrics.With(prometheus.Labels{"node": nodeName}).Set(disabled)
}

// SLONotMetCounterInc increases sloNotMetCounter number
func SLONotMetCounterInc(appName string) {
	nodeName := util.NodeIP()
	sloNotMetCounter := totalMetrics[metricsNameSLONotMetCounter].(*prometheus.CounterVec)
	sloNotMetCounter.WithLabelValues(nodeName, appName).Inc()
}

// OnlineJobsMetrics records online job metric data
func OnlineJobsMetrics(jobName string, metrics map[string]float64) {
	nodeName := util.NodeIP()
	onlineJobsMetrics := totalMetrics[metricsNameOnlineJob].(*prometheus.GaugeVec)
	for k, v := range metrics {
		onlineJobsMetrics.WithLabelValues(nodeName, jobName, k).Set(v)
	}
}

// DiskSpaceMetrics record disk space metrics data
func DiskSpaceMetrics(diskStats map[string]*global.DiskPartitionStats) {
	nodeName := util.NodeIP()
	diskSpaceMetric := totalMetrics[metricsDiskSpace].(*prometheus.GaugeVec)
	for mountpoint, stat := range diskStats {
		diskSpaceMetric.WithLabelValues(nodeName, "total", mountpoint).Set(float64(stat.TotalSize / types.DiskUnit))
		diskSpaceMetric.WithLabelValues(nodeName, "free", mountpoint).Set(float64(stat.FreeSize / types.DiskUnit))
	}
}

// DiskSpaceLimited counts if the node has little disk space, and cannot run offline job
func DiskSpaceLimited(spaceLimited float64) {
	nodeName := util.NodeIP()
	diskSpaceLimitedMetrics := totalMetrics[metricsNameDiskSpaceLimited].(*prometheus.GaugeVec)
	diskSpaceLimitedMetrics.With(prometheus.Labels{"node": nodeName}).Set(spaceLimited)
}

// AlarmCounterInc increases alarm metric counter
func AlarmCounterInc() {
	alarmMetrics := totalMetrics[metricsAlarmCounter].(*prometheus.CounterVec)
	alarmMetrics.WithLabelValues(util.NodeIP()).Inc()
}
