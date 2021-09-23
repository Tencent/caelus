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
	"github.com/tencent/caelus/pkg/caelus/diskquota"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"k8s.io/apimachinery/pkg/util/sets"
)

type StatsMetric string

var (
	StatsMetricStore     StatsMetric = "store"
	StatsMetricDiskQuota StatsMetric = "diskquota"
)

// resourceMetricsCollector describe options for prometheus metrics
type resourceMetricsCollector struct {
	nodeName    string
	statsMetric map[StatsMetric]interface{}
	metrics     ResourceMetrics
	errors      prometheus.Gauge
}

// NewPrometheusCollector new prometheus collector
func NewPrometheusCollector(nodeName string, statsMetric map[StatsMetric]interface{},
	metrics ResourceMetrics) prometheus.Collector {

	return &resourceMetricsCollector{
		nodeName:    nodeName,
		statsMetric: statsMetric,
		metrics:     metrics,
		errors: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "scrape_error",
			Help: "1 if there was an error while getting container metrics, 0 otherwise",
		}),
	}
}

var _ prometheus.Collector = &resourceMetricsCollector{}

// Describe implements prometheus.Collector
func (rc *resourceMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	rc.errors.Describe(ch)
	// node metrics
	for _, metric := range rc.metrics.NodeMetrics {
		ch <- metric.desc()
	}
	// container metrics
	for _, metric := range rc.metrics.ContainerMetrics {
		ch <- metric.desc()
	}
	// perf metrics
	for _, metric := range rc.metrics.PerfMetrics {
		ch <- metric.desc()
	}
	// RDT metrics
	for _, metric := range rc.metrics.RdtMetrics {
		ch <- metric.desc()
	}
	// disk quota metrics
	for _, metric := range rc.metrics.DiskQuotaMetrics {
		ch <- metric.desc()
	}
}

// Collect implements prometheus.Collector
func (rc *resourceMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	rc.errors.Set(0)
	defer rc.errors.Collect(ch)

	// check if supported firstly
	storeOk := rc.statsMetric[StatsMetricStore]
	if storeOk != nil {
		rc.collectNodeMetrics(ch)
		rc.collectContainerMetrics(ch)
		rc.collectPerfMetrics(ch)
		rc.collectRdtMetrics(ch)
	}
	diskQuotaOk := rc.statsMetric[StatsMetricDiskQuota]
	if diskQuotaOk != nil {
		rc.collectDiskQuotaMetrics(ch)
	}
}

// collectNodeMetrics collect metrics for node
func (rc *resourceMetricsCollector) collectNodeMetrics(ch chan<- prometheus.Metric) {
	stStore := rc.statsMetric[StatsMetricStore].(statestore.StateStore)
	nodeStats, err := stStore.GetNodeResourceRecentState()
	if err != nil {
		return
	}

	for _, metric := range rc.metrics.NodeMetrics {
		var values []metricValue
		// node level resource usage
		if metric.ValueFn != nil {
			values = append(values, metric.ValueFn(nodeStats)...)
		}
		// job level resource usage, such online/offline
		if metric.ValueFn2 != nil {
			values = append(values, metric.ValueFn2(stStore)...)
		}

		for _, value := range values {
			ch <- prometheus.NewMetricWithTimestamp(value.timestamp,
				prometheus.MustNewConstMetric(metric.desc(), prometheus.GaugeValue,
					value.value, append([]string{rc.nodeName}, value.labels...)...))
		}
	}
}

// collectContainerMetrics collect metrics for cgroup
func (rc *resourceMetricsCollector) collectContainerMetrics(ch chan<- prometheus.Metric) {
	stStore := rc.statsMetric[StatsMetricStore].(statestore.StateStore)

	containerStats, err := stStore.ListCgroupResourceRecentState(false, sets.NewString())
	if err != nil {
		return
	}
	for _, cs := range containerStats {
		for _, metric := range rc.metrics.ContainerMetrics {
			if values := metric.ValueFn(cs); len(values) != 0 {
				for _, value := range values {
					ch <- prometheus.NewMetricWithTimestamp(value.timestamp,
						prometheus.MustNewConstMetric(metric.desc(), prometheus.GaugeValue, value.value,
							append([]string{cs.Ref.ContainerName, cs.Ref.PodName, cs.Ref.PodNamespace, rc.nodeName,
								string(cs.Ref.AppClass)}, value.labels...)...))
				}
			}
		}
	}
}

// collectPerfMetrics collect metrics for performance counters
func (rc *resourceMetricsCollector) collectPerfMetrics(ch chan<- prometheus.Metric) {
	stStore := rc.statsMetric[StatsMetricStore].(statestore.StateStore)

	containerPerfMetrics, err := stStore.ListPerfResourceRecentStats()
	if err != nil {
		return
	}
	for _, cp := range containerPerfMetrics {
		for _, metric := range rc.metrics.PerfMetrics {
			if values := metric.ValueFn(cp); len(values) != 0 {
				for _, value := range values {
					ch <- prometheus.NewMetricWithTimestamp(value.timestamp,
						prometheus.MustNewConstMetric(metric.desc(), prometheus.GaugeValue, value.value,
							append([]string{cp.Spec.ContainerName, cp.Spec.PodName, cp.Spec.PodNamespace},
								value.labels...)...))
				}
			}
		}
	}
}

// collectRdtMetrics collect metrics for Resource Director technology
func (rc *resourceMetricsCollector) collectRdtMetrics(ch chan<- prometheus.Metric) {
	stStore := rc.statsMetric[StatsMetricStore].(statestore.StateStore)

	containerRdtMetrics, err := stStore.ListRDTResourceRecentStats()
	if err == nil {
		for _, cp := range containerRdtMetrics {
			for _, metric := range rc.metrics.RdtMetrics {
				if values := metric.ValueFn(cp); len(values) != 0 {
					for _, value := range values {
						ch <- prometheus.NewMetricWithTimestamp(value.timestamp,
							prometheus.MustNewConstMetric(metric.desc(), prometheus.GaugeValue, value.value,
								append([]string{cp.Spec.ContainerName, cp.Spec.PodName, cp.Spec.PodNamespace},
									value.labels...)...))

					}
				}
			}
		}
	}
}

// collectDiskQuotaMetrics collect metrics for disk quota, such as disk space and inodes
func (rc *resourceMetricsCollector) collectDiskQuotaMetrics(ch chan<- prometheus.Metric) {
	stat := rc.statsMetric[StatsMetricDiskQuota].(diskquota.DiskQuotaInterface)
	podQuotas, err := stat.GetAllPodsDiskQuota()
	if err != nil {
		return
	}
	for _, podVolumes := range podQuotas {
		for _, metric := range rc.metrics.DiskQuotaMetrics {
			if values := metric.ValueFn(podVolumes); len(values) != 0 {
				for _, value := range values {
					ch <- prometheus.NewMetricWithTimestamp(value.timestamp,
						prometheus.MustNewConstMetric(metric.desc(), prometheus.GaugeValue, value.value,
							append([]string{podVolumes.Pod.Name, podVolumes.Pod.Namespace, rc.nodeName,
								string(podVolumes.AppClass)}, value.labels...)...))
				}
			}
		}
	}
}
