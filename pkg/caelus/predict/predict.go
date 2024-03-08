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
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/machine"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

// predictorWrapper implements predictor interface. It calculates allocatable resource for offline pods
// based on predict resource of oneline/system pods, machine capacity and reserved resource.
type predictorWrapper struct {
	types.PredictConfig
	// If multiple predicts, the first one is used for real prediction. The left are experiment predicts, caelus will
	// only feeds samples to them and expose predict metrics for them.
	predictors   []Predictor
	metricsTypes []string

	cacheLock sync.Mutex
	cacheRes  v1.ResourceList
	cacheTs   time.Time
}

// module name
func (p *predictorWrapper) Name() string {
	return "ModulePredictor"
}

// Run starts all predictors
func (p *predictorWrapper) Run(stop <-chan struct{}) {
	for i := range p.predictors {
		p.predictors[i].AddSample(stop)
	}
}

// GetAllocatableForBatch returns allocatable resources for offline jobs
func (p *predictorWrapper) GetAllocatableForBatch() v1.ResourceList {
	p.cacheLock.Lock()
	defer p.cacheLock.Unlock()
	if len(p.cacheRes) > 0 && time.Since(p.cacheTs) < p.CheckInterval.TimeDuration() {
		return p.cacheRes.DeepCopy()
	}
	var onlinePredict v1.ResourceList
	for i := 0; i < len(p.predictors); i++ {
		predicted := p.predictors[i].Predict()
		if i == 0 {
			onlinePredict = predicted
		}
		klog.V(3).Infof("[%s] predicted: milli cpu %d, mem %dM)", p.metricsTypes[i],
			predicted.Cpu().MilliValue(), predicted.Memory().Value()/types.MemUnit)
		metrics.NodeResourceMetricsReset(predicted, p.metricsTypes[i])
	}

	// node total resource
	total, err := machine.GetTotalResource()
	if err != nil {
		klog.Errorf("get total resource err: %v", err)
		return nil
	}
	klog.V(5).Infof("total resource: %+v", total)
	metrics.NodeResourceMetricsReset(total, metrics.NodeResourceTypeTotal)

	// reserved resource
	reserved := v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(int64(*p.ReserveResource.CpuMilli), resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(int64(*p.ReserveResource.MemMB)*types.MemUnit, resource.DecimalSI),
	}
	klog.V(5).Infof("reserved resource: %v", reserved)
	metrics.NodeResourceMetricsReset(reserved, metrics.NodeResourceTypeBuffer)

	totalQuan := total[v1.ResourceCPU]
	reservedQuan := reserved[v1.ResourceCPU]
	cpu := onlinePredict.Cpu().MilliValue()
	klog.V(2).Infof("local predict cpu stats: total(%d), predict(%d), reserved(%d)",
		totalQuan.MilliValue(), cpu, reservedQuan.MilliValue())
	cpu = totalQuan.MilliValue() - cpu - reservedQuan.MilliValue()
	if cpu < 0 {
		klog.Warningf("negative cpu predicted value, set it as zero, total: %d, recommended: %d, reserved: %d",
			totalQuan.MilliValue(), cpu, reservedQuan.MilliValue())
		cpu = 0
	}

	totalQuan = total[v1.ResourceMemory]
	reservedQuan = reserved[v1.ResourceMemory]
	mem := onlinePredict.Memory().Value()
	klog.V(2).Infof("local predict memory stats: total(%d), predict(%d), reserved(%d)",
		totalQuan.Value()/types.MemUnit, mem/types.MemUnit, reservedQuan.Value()/types.MemUnit)
	mem = totalQuan.Value() - mem - reservedQuan.Value()
	if mem < 0 {
		klog.Warningf("negative memory predicted value, set it as zero, total: %d, recommended: %d, reserved: %d",
			totalQuan.Value(), mem, reservedQuan.Value())
		mem = 0
	}

	rl := v1.ResourceList{}
	cpuQ := resource.NewMilliQuantity(cpu, resource.DecimalSI)
	memQ := resource.NewQuantity(mem, resource.DecimalSI)
	rl[v1.ResourceCPU] = *cpuQ
	rl[v1.ResourceMemory] = *memQ
	metrics.NodeResourceMetricsReset(rl, metrics.NodeResourceTypeOfflinePredict)
	p.cacheTs = time.Now()
	p.cacheRes = rl
	return rl.DeepCopy()
}

// GetReservedResource output reserved resource quantity
func (p *predictorWrapper) GetReservedResource() v1.ResourceList {
	return v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(int64(*p.ReserveResource.CpuMilli), resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(int64(*p.ReserveResource.MemMB)*types.MemUnit, resource.DecimalSI),
	}
}

// NewPredictorOrDie creates a new predictor
func NewPredictorOrDie(configs []types.PredictConfig, input statestore.StateStore) Interface {
	if len(configs) == 0 {
		klog.Fatalf("empty predict config")
	}
	var predictors []Predictor
	var metricsTypes []string
	for i := range configs {
		predictors = append(predictors, newPredictor(configs[i], input))
		metricsTypes = append(metricsTypes, configs[i].PredictMetricsType)
	}
	return &predictorWrapper{
		PredictConfig: configs[0],
		predictors:    predictors,
		metricsTypes:  metricsTypes,
	}
}

func newPredictor(config types.PredictConfig, input statestore.StateStore) Predictor {
	switch config.PredictType {
	case types.LocalPredictorType:
		return NewLocalPredict(config, input)
	case types.VPAPredictorType:
		return NewVpaPredictOrDie(config)
	default:
		klog.Fatalf("unknown predict type: %s", config.PredictType)
	}
	return nil
}
