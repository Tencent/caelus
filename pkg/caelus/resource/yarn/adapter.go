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
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/types"
	global "github.com/tencent/caelus/pkg/types"
	"github.com/tencent/caelus/pkg/util/times"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
	"time"
)

// ResourceAdapter is the interface to format offline resource list
type ResourceAdapter interface {
	// ResourceAdapt format resource list, and return the bool value indicating if reaching the minimum value
	ResourceAdapt(resList v1.ResourceList) bool
	// Run start main loop
	Run(stopCh <-chan struct{})
}

// minCompareAdapter describe options for compare with min capacity adapter
type minCompareAdapter struct {
	ginit GInitInterface
}

// NewMinCompareAdapter new a adapter for comparing with min capacity
func NewMinCompareAdapter(ginit GInitInterface) ResourceAdapter {
	return &minCompareAdapter{ginit: ginit}
}

// Run do nothing for now
func (m *minCompareAdapter) Run(stopCh <-chan struct{}) {}

// ResourceAdapt compare with min capacity
func (m *minCompareAdapter) ResourceAdapt(resList v1.ResourceList) bool {
	return compareAndReplaceMinCapacity(resList, m.ginit.GetMinCapacity)
}

// reserveAdapter reserve some resource for nm
type reserveAdapter struct {
	reserve types.Resource
	ginit   GInitInterface
}

// ResourceAdapt reserve some resource for nm
func (r *reserveAdapter) ResourceAdapt(res v1.ResourceList) bool {
	if r.reserve.MemMB != nil {
		res[v1.ResourceMemory] = *resource.NewQuantity(res.Memory().Value()-int64(*r.reserve.MemMB)*types.MemUnit,
			resource.DecimalSI)
	}
	if r.reserve.CpuMilli != nil {
		res[v1.ResourceCPU] = *resource.NewMilliQuantity(res.Cpu().MilliValue()-int64(*r.reserve.CpuMilli),
			resource.DecimalSI)
	}
	return compareAndReplaceMinCapacity(res, r.ginit.GetMinCapacity)
}

// Run do nothing for now
func (r *reserveAdapter) Run(stopCh <-chan struct{}) {}

// NewResourceReserveAdapter new an adapter for reserve resources
func NewResourceReserveAdapter(resource types.Resource, ginit GInitInterface) ResourceAdapter {
	return &reserveAdapter{reserve: resource, ginit: ginit}
}

// overcommitAdapter is an adapter for overcommit cpu resource
type overcommitAdapter struct {
	overCommit types.OverCommit
	ginit      GInitInterface
}

// ResourceAdapt overcommit resources based on current usage
func (o *overcommitAdapter) ResourceAdapt(res v1.ResourceList) bool {
	if !o.overCommit.Enable {
		return false
	}
	percent := o.overCommit.OverCommitPercent
	now := time.Now()
	for _, p := range o.overCommit.Periods {
		if times.IsTimeInSecondsDay(now, p.Range) {
			percent = p.OverCommitPercent
			break
		}
	}
	overcommitCpu := float64(res.Cpu().MilliValue()) * percent
	res[v1.ResourceCPU] = *resource.NewMilliQuantity(int64(overcommitCpu), resource.DecimalSI)
	return compareAndReplaceMinCapacity(res, o.ginit.GetMinCapacity)
}

// Run do nothing for now
func (o *overcommitAdapter) Run(stopCh <-chan struct{}) {}

// NewOverCommitAdapter new an overcommit adapter for cpu resource
func NewOverCommitAdapter(overcommit types.OverCommit, ginit GInitInterface) ResourceAdapter {
	return &overcommitAdapter{overCommit: overcommit, ginit: ginit}
}

// roundOffAdapter describe options for rounding off resource list
type roundOffAdapter struct {
	ginit            GInitInterface
	resourceRoundOff types.RoundOffResource
}

// NewRoundOffAdapter new a roundOff adapter instance
func NewRoundOffAdapter(resRoundOff types.RoundOffResource, ginit GInitInterface) ResourceAdapter {
	return &roundOffAdapter{
		resourceRoundOff: resRoundOff,
		ginit:            ginit,
	}
}

// ResourceAdapt describe how to round off resource list
// rounding off resource list could reduce noise
func (o *roundOffAdapter) ResourceAdapt(resList v1.ResourceList) bool {
	memMb := resList.Memory().Value() / types.MemUnit
	cpu := resList.Cpu().MilliValue()

	roundMem := o.resourceRoundOff.MemMB
	roundCpu := o.resourceRoundOff.CPUMilli
	if roundMem != 0 {
		memMb = int64(float64(memMb)/roundMem) * int64(roundMem)
	}
	if roundCpu != 0 {
		cpu = int64(float64(cpu)/roundCpu) * int64(roundCpu)
	}

	resList[v1.ResourceCPU] = *resource.NewMilliQuantity(int64(cpu), resource.DecimalSI)
	resList[v1.ResourceMemory] = *resource.NewQuantity(int64(memMb*types.MemUnit), resource.DecimalSI)
	klog.V(4).Infof("sync node resource, after round off: %+v", resList)

	return compareAndReplaceMinCapacity(resList, o.ginit.GetMinCapacity)
}

// Run do nothing
func (o *roundOffAdapter) Run(stopCh <-chan struct{}) {
}

// diskCpuAdapter will compare offline cores with disk space, and choose the little one
type diskCpuAdapter struct {
	ginit GInitInterface
	disk  *DiskManager
}

// NewDiskCpuAdapter new a diskCpu adapter instance
func NewDiskCpuAdapter(disk *DiskManager, ginit GInitInterface) ResourceAdapter {
	return &diskCpuAdapter{
		disk:  disk,
		ginit: ginit,
	}
}

// ResourceAdapt choose the little one between offline cores and disk space
func (d *diskCpuAdapter) ResourceAdapt(resList v1.ResourceList) bool {
	if d.disk.RatioToCore == nil {
		return compareAndReplaceMinCapacity(resList, d.ginit.GetMinCapacity)
	}
	diskCores, err := d.disk.DiskSpaceToCores()
	if err != nil {
		klog.Errorf("disk space to cpus err: %v", err)
		return compareAndReplaceMinCapacity(resList, d.ginit.GetMinCapacity)
	}

	klog.V(4).Infof("compare cpus based on disk space: %+v", diskCores)
	diskCpus := *diskCores
	cpu := resList[v1.ResourceCPU].DeepCopy()
	if cpu.Cmp(diskCpus) > 0 {
		cpu.Sub(diskCpus)
		klog.V(2).Infof("cpu resource changed to %v based on disk space", diskCpus)
		resList[v1.ResourceCPU] = diskCpus
	} else {
		cpu.Sub(cpu)
	}
	metrics.NodeResourceMetricsReset(v1.ResourceList{v1.ResourceCPU: cpu}, metrics.NodeResourceTypeOfflineDisks)

	klog.V(4).Infof("sync node resource, after disk cores: %+v", resList)

	return compareAndReplaceMinCapacity(resList, d.ginit.GetMinCapacity)
}

// Run start disk manager thread
func (d *diskCpuAdapter) Run(stopCh <-chan struct{}) {
	d.disk.Run(stopCh)
}

// compareAndReplaceMinCapacity change to min value if less than min capacity
func compareAndReplaceMinCapacity(res v1.ResourceList, minFunc func() *global.NMCapacity) bool {
	var reachMin bool

	minCap := minFunc()
	if res.Memory().Value()/types.MemUnit <= minCap.MemoryMB {
		res[v1.ResourceMemory] = *resource.NewQuantity(minCap.MemoryMB*types.MemUnit, resource.DecimalSI)
		reachMin = true
	}
	if res.Cpu().MilliValue() <= minCap.Millcores {
		res[v1.ResourceCPU] = *resource.NewMilliQuantity(minCap.Millcores, resource.DecimalSI)
		reachMin = true
	}

	return reachMin
}
