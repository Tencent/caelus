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

package yarn

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tencent/caelus/pkg/caelus/alarm"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	global "github.com/tencent/caelus/pkg/types"

	"github.com/parnurzeal/gorequest"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

const (
	// nodemanager metrics url pattern
	metricsSuffix = "/jmx?qry=Hadoop:service=NodeManager,name=NodeManagerMetrics"
	// node info url pattern
	statusSuffix = "/ws/v1/node/info"
	// container list url pattern
	containersSuffix = "/ws/v1/node/containers"
	warningPeriod    = time.Duration(10 * time.Minute)
)

// YarnMetrics collect yarn nodemanager metrics for the node
type YarnMetrics struct {
	nodeName            string
	containerStatusDesc *prometheus.Desc
	nodeStatusDesc      *prometheus.Desc
	client              *gorequest.SuperAgent
	url                 unsafe.Pointer
	metrics             *NMMetrics
	nodeStatus          *NMNodeInfo
	metricsLock         sync.Mutex
	metricsPortChan     chan int
	lastFailed          int
	lastKilled          int
	lastCompleted       int
	lastWarning         time.Time
	failedCount         int
}

// NewYarnMetrics create a yarnMetrics instance
func NewYarnMetrics(metricsPort int, metricsPortChan chan int) *YarnMetrics {
	m := &YarnMetrics{
		nodeName: util.NodeIP(),
		containerStatusDesc: prometheus.NewDesc("caelus_yarn_nm_container", "nodemanager containers status",
			[]string{"node", "type"}, nil),
		nodeStatusDesc: prometheus.NewDesc("caelus_yarn_nm_healthy", "nodemanager node healthy status",
			[]string{"node"}, nil),
		client:          gorequest.New().SetDebug(bool(klog.V(4))).Timeout(time.Second * 5),
		metricsPortChan: metricsPortChan,
		lastWarning:     time.Now().Add(-warningPeriod),
		failedCount:     0,
	}
	m.resetMetricUrl(metricsPort)
	return m
}

func (y *YarnMetrics) resetMetricUrl(metricsPort int) {
	url := fmt.Sprintf("http://%s:%d", y.nodeName, metricsPort)
	if c := (*string)(atomic.LoadPointer(&y.url)); c != nil && *c == url {
		return
	}
	klog.V(2).Infof("yarn metrics url %s", url)
	atomic.StorePointer(&y.url, unsafe.Pointer(&url))
}

// Run start collecting yarn metrics and watch for metrics port changing in a new goroutine
func (y *YarnMetrics) Run(stop <-chan struct{}) {
	go wait.Until(func() {
		func() {
			y.metricsLock.Lock()
			defer y.metricsLock.Unlock()
			// nodemanager metrics
			y.metrics = y.getMetrics()
			// nodemanager health status
			y.nodeStatus = y.getNodeStatus()
		}()
		y.uploadNodeMetrics()
		// warning if too many failed jobs
		y.checkAbnormalFailed()

	}, time.Second*30, stop)

	// get yarn metrics from channel
	if y.metricsPortChan != nil {
		go func() {
			for {
				select {
				case <-stop:
					return
				case port := <-y.metricsPortChan:
					y.resetMetricUrl(port)
				}
			}
		}()
	}
}

// Describe put metrics descs to prometheus.Desc chan
func (y *YarnMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- y.containerStatusDesc
	ch <- y.nodeStatusDesc
}

// Collect put metrics to prometheus.Metric chan
func (y *YarnMetrics) Collect(ch chan<- prometheus.Metric) {
	y.metricsLock.Lock()
	defer y.metricsLock.Unlock()
	if y.metrics != nil {
		ch <- prometheus.MustNewConstMetric(y.containerStatusDesc, prometheus.GaugeValue,
			float64(y.metrics.ContainersLaunched), y.nodeName, "launched")
		ch <- prometheus.MustNewConstMetric(y.containerStatusDesc, prometheus.GaugeValue,
			float64(y.metrics.ContainersCompleted), y.nodeName, "completed")
		ch <- prometheus.MustNewConstMetric(y.containerStatusDesc, prometheus.GaugeValue,
			float64(y.metrics.ContainersFailed), y.nodeName, "failed")
		ch <- prometheus.MustNewConstMetric(y.containerStatusDesc, prometheus.GaugeValue,
			float64(y.metrics.ContainersKilled), y.nodeName, "killed")
		ch <- prometheus.MustNewConstMetric(y.containerStatusDesc, prometheus.GaugeValue,
			float64(y.metrics.ContainersRunning), y.nodeName, "running")
		ch <- prometheus.MustNewConstMetric(y.containerStatusDesc, prometheus.GaugeValue,
			float64(y.metrics.ContainersIniting), y.nodeName, "initing")
	}
	if y.nodeStatus != nil {
		ch <- prometheus.MustNewConstMetric(y.nodeStatusDesc, prometheus.GaugeValue,
			y.nodeStatus.NodeHealthyFloat, y.nodeName)
	}
}

func (y *YarnMetrics) uploadNodeMetrics() {
	// update allocated resource
	// The yarn metrics have contains the allocated resource, but it not works normally sometimes,
	// so we just calculate the value from running containers, this will be dropped when bug fixed.
	var cpu, mem int64
	containers := y.getContainerState()
	for _, con := range containers {
		if runningStats.Union(initingStats).Has(con.State) {
			cpu += int64(con.TotalVCoresNeeded)
			mem += int64(con.TotalMemoryNeededMB)
		}
	}
	alloc := v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(cpu*types.CpuUnit, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(mem*types.MemUnit, resource.DecimalSI),
	}
	metrics.NodeResourceMetricsReset(alloc, metrics.NodeResourceTypeOfflineAllocated)

	// update capacity, this value is dependable
	if y.metrics != nil {
		cpu = 0
		mem = 0
		cpu = y.metrics.AvailableVCores + y.metrics.AllocatedVCores
		mem = y.metrics.AvailableGB + y.metrics.AllocatedGB
		cap := v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(cpu*types.CpuUnit, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(mem*types.MemGbUnit, resource.DecimalSI),
		}
		metrics.NodeResourceMetricsReset(cap, metrics.NodeResourceTypeOfflineCapacity)
	}
}

// getMetrics request nodemanager metrics
func (y *YarnMetrics) getMetrics() *NMMetrics {
	jmxResp := &JmxResp{}
	err := y.requestUrl(metricsSuffix, jmxResp)
	if err != nil {
		klog.V(5).Infof("request nodemanager metrics err: %v", err)
		return nil
	}
	if len(jmxResp.Beans) == 0 {
		klog.V(5).Infof("request nodemanager metrics, empty beans")
		return nil
	}
	return &jmxResp.Beans[0]
}

// checkAbnormalFailed check if the failed containers number is increasing frequently, and send alarm
func (y *YarnMetrics) checkAbnormalFailed() {
	if y.metrics == nil {
		return
	}
	currentFailed := y.metrics.ContainersFailed
	currentKilled := y.metrics.ContainersKilled
	currentCompleted := y.metrics.ContainersCompleted

	failed := currentFailed - y.lastFailed
	killed := currentKilled - y.lastKilled
	completed := currentCompleted - y.lastCompleted
	if killed < 0 || completed < 0 {
		// may be nodemanager has restarted
		klog.Warningf("found negative value(failed:%d-%d, killed:%d-%d, completed:%d-%d), using current value",
			currentFailed, y.lastFailed, currentKilled, y.lastKilled, currentCompleted, y.lastCompleted)
		failed = currentFailed
		killed = currentKilled
		completed = currentCompleted
	}

	if failed > 1 && failed > killed && failed > completed {
		y.failedCount++
		// the "3" may be not very reasonable, and users could change the value depending on different loads
		if y.failedCount >= 3 {
			msg := fmt.Sprintf("too many offline job failed(failed:%d, killed:%d, completed:%d)",
				failed, killed, completed)
			klog.Errorf(msg)
			if y.lastWarning.Add(warningPeriod).Before(time.Now()) {
				alarm.SendAlarm(msg)
				y.lastWarning = time.Now()
			}
		}
	} else {
		y.failedCount = 0
	}

	y.lastFailed = currentFailed
	y.lastKilled = currentKilled
	y.lastCompleted = currentCompleted
}

// getNodeStatus request nodemanager status
func (y *YarnMetrics) getNodeStatus() *NMNodeInfo {
	nmNodeStatus := &NMNodeStatus{}
	err := y.requestUrl(statusSuffix, nmNodeStatus)
	if err != nil {
		klog.Errorf("request nodemanager status err: %v", err)
		// node is in lost state
		nmNodeStatus.NodeInfo = &NMNodeInfo{
			NodeHealthy: false,
		}
	}
	if nmNodeStatus.NodeInfo.NodeHealthy {
		nmNodeStatus.NodeInfo.NodeHealthyFloat = 1
	} else {
		nmNodeStatus.NodeInfo.NodeHealthyFloat = 0
	}
	return nmNodeStatus.NodeInfo
}

// getContainerState request container list
func (y *YarnMetrics) getContainerState() []global.NMContainer {
	containerWrapper := &global.NMContainersWrapper{}
	err := y.requestUrl(containersSuffix, containerWrapper)
	if err != nil {
		klog.Errorf("request containers err: %v", err)
		return []global.NMContainer{}
	}

	return containerWrapper.NMContainers.Container
}

func (y *YarnMetrics) requestUrl(urlSuffix string, respSturct interface{}) error {
	var url string
	if c := (*string)(atomic.LoadPointer(&y.url)); c != nil {
		url = *c + urlSuffix
		klog.V(5).Infof("curl nodemanager: %s", url)
	} else {
		return fmt.Errorf("nodemanager url not found")
	}
	resp, _, errs := y.client.Get(url).EndStruct(respSturct)
	if len(errs) != 0 {
		return fmt.Errorf("%v", errs)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response status not ok: %v", resp.StatusCode)
	}

	return nil
}
