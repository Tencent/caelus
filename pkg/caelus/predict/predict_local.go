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

package predict

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tencent/caelus/pkg/caelus/detection"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/model"
	"k8s.io/klog/v2"
)

// localPredict predicts for online processes on current node.
// It assumes online processes run on machine and all pods running on k8s are batch jobs.
type localPredict struct {
	types.PredictConfig
	stStore         statestore.StateStore
	recommender     PodResourceRecommender
	statMap         model.ContainerNameToAggregateStateMap
	statMapLock     sync.Mutex
	addSampleTimes  *int32
	initSampleTimes int32
	lastPrintTime1  time.Time
	lastPrintTime2  time.Time
	anomalyDetector *detection.EwmaDetector
}

// Predict predicts resources used by online and system pods
func (p *localPredict) Predict() v1.ResourceList {
	var cpu, mem int64

	// if predict is disabled, just return zero quantity
	if p.Disable {
		return v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(cpu, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(mem, resource.DecimalSI),
		}
	}

	// wait for local predictor charging before predicting resource
	_ = wait.PollImmediate(time.Second*5, time.Minute*2, func() (bool, error) {
		if atomic.LoadInt32(p.addSampleTimes) >= p.initSampleTimes {
			return true, nil
		}
		klog.V(2).Infof("[%s] waiting for local predictor charging", p.PredictMetricsType)
		return false, nil
	})
	p.statMapLock.Lock()
	defer p.statMapLock.Unlock()
	// print log if v3 or print time
	predictLog := klog.V(4)
	now := time.Now()
	if p.lastPrintTime1.Add(p.PrintInterval.TimeDuration()).Before(now) {
		p.lastPrintTime1 = now
		predictLog = klog.V(0)
	}
	// recommended resource
	recommended := p.recommender.GetRecommendedPodResources(p.statMap)
	for name, re := range recommended {
		cpu += int64(re.Target[model.ResourceCPU])
		mem += int64(re.Target[model.ResourceMemory])
		predictLog.Infof("[%s] %s recommend target(%d,%d)", p.PredictMetricsType, name,
			re.Target[model.ResourceCPU], re.Target[model.ResourceMemory])
	}
	return v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(cpu, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(mem, resource.DecimalSI),
	}
}

// AddSample starts adding new sample data periodically
func (p *localPredict) AddSample(stop <-chan struct{}) {
	// no need to run if predict is disabled
	if !p.Disable {
		go wait.Until(p.addSample, p.CheckInterval.TimeDuration(), stop)
	} else {
		klog.Warningf("[%s] predict is disabled, no need to collect sample", p.PredictMetricsType)
	}
}

// getRecentOnlineResource get recent online job resource usage
func (p *localPredict) getRecentOnlineResource() (float64, float64, error) {
	nodeSt, err := p.stStore.GetNodeResourceRecentState()
	if err != nil || nodeSt == nil || nodeSt.CPU == nil || nodeSt.Memory == nil {
		return 0, 0, fmt.Errorf("no node state")
	}
	cpuStats := nodeSt.CPU
	memStats := nodeSt.Memory

	klog.V(4).Infof("predict sample, total(%f, %f), offline(%f, %f)",
		cpuStats.CpuTotal, memStats.UsageRss, cpuStats.CpuOfflineTotal, memStats.OfflineTotal)
	milliCPU := math.Max(cpuStats.CpuTotal-cpuStats.CpuOfflineTotal, 0) * 1000
	mem := math.Max(memStats.UsageRss-memStats.OfflineTotal, 0)
	return milliCPU, mem, nil
}

func (p *localPredict) addSample() {
	milliCPU, mem, err := p.getRecentOnlineResource()
	if err != nil {
		klog.Errorf("addSample failed: %v", err)
		return
	}
	if atomic.LoadInt32(p.addSampleTimes) < p.initSampleTimes {
		atomic.AddInt32(p.addSampleTimes, 1)
	}
	addSampleLog := klog.V(4)
	now := time.Now()
	if p.lastPrintTime2.Add(p.PrintInterval.TimeDuration()).Before(now) {
		p.lastPrintTime2 = now
		addSampleLog = klog.V(0)
	}

	// take kernel usage into account
	name := "online-system"
	cpuWeight := model.ResourceAmount(100) // = model.minSampleWeight*1000
	if p.EnableTuneCPUWeight {
		p.anomalyDetector.Add(detection.TimedData{Ts: now, Vals: map[string]float64{"cpu": milliCPU}})
		if isA, err := p.anomalyDetector.IsAnomaly(); err == nil && isA {
			if milliCPU > p.anomalyDetector.Mean() {
				// cpu increase too rapidly, increase weight
				cpuWeight = model.ResourceAmount(p.IncreasingAnomalyWeightFactor) * cpuWeight
			} else {
				// cpu decrease too rapidly, increase weight, but don't add more weight than cpu increasing rapidly
				cpuWeight = model.ResourceAmount(p.DecreasingAnomalyWeightFactor) * cpuWeight
			}
			klog.Infof("[%s] detect cpu usage %f anomaly, mean %f stdDev %f, setting cpu weight %v",
				p.PredictMetricsType, milliCPU, p.anomalyDetector.Mean(), p.anomalyDetector.StdDev(), cpuWeight)
		}
	}
	acs := p.getOrCreateACS(name)
	acs.AddSample(&model.ContainerUsageSample{
		MeasureStart: now,
		Usage:        model.ResourceAmount(int64(milliCPU)),
		Request:      cpuWeight,
		Resource:     model.ResourceCPU,
	})
	acs.AddSample(&model.ContainerUsageSample{
		MeasureStart: now,
		Usage:        model.ResourceAmount(int64(mem)),
		Request:      0,
		Resource:     model.ResourceMemory,
	})
	addSampleLog.Infof("[%s] %s sample cpu %f cpuWeight %v, mem %f, time %v, total sample %d",
		p.PredictMetricsType, name, milliCPU, cpuWeight, mem, now, acs.TotalSamplesCount)
	addSampleLog.Infof("[%s] %s cpu histogram %v", p.PredictMetricsType, name, acs.AggregateCPUUsage)
	addSampleLog.Infof("[%s] %s memory histogram %v", p.PredictMetricsType, name, acs.AggregateMemoryPeaks)
}

func (p *localPredict) getOrCreateACS(containerName string) *model.AggregateContainerState {
	p.statMapLock.Lock()
	defer p.statMapLock.Unlock()
	acs, ok := p.statMap[containerName]
	if !ok {
		klog.Infof("[%s] create acs for %s", p.PredictMetricsType, containerName)
		acs = model.NewAggregateContainerState()
		p.statMap[containerName] = acs
	}
	return acs
}

// NewLocalPredict creates a local predictor
func NewLocalPredict(config types.PredictConfig, input statestore.StateStore) Predictor {
	model.InitializeAggregationsConfig(model.NewAggregationsConfig(time.Duration(config.MemoryAggregationInterval),
		config.MemoryAggregationIntervalCount, time.Duration(config.MemoryHistogramDecayHalfLife),
		time.Duration(config.CPUHistogramDecayHalfLife)))
	var sampleTimes int32
	minData := int(time.Minute * 30 / config.CheckInterval.TimeDuration())
	if minData < detection.MinData {
		minData = detection.MinData
	}
	return &localPredict{
		recommender:     CreatePodResourceRecommender(config.LocalPredictConfig),
		statMap:         model.ContainerNameToAggregateStateMap{},
		stStore:         input,
		PredictConfig:   config,
		addSampleTimes:  &sampleTimes,
		initSampleTimes: 3,
		anomalyDetector: detection.NewEwmaDetector("cpu", minData),
	}
}
