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

package qos

import (
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"time"

	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/online"
	"github.com/tencent/caelus/pkg/caelus/predict"
	"github.com/tencent/caelus/pkg/caelus/qos/manager"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// QosK8sManager sets quota limit for offline jobs, limited on:
// - online job on k8s
// - offline job on k8s
// - offline jon yarn on k8s
type QosK8sManager struct {
	predict  predict.Interface
	stStore  statestore.StateStore
	conflict conflict.Manager
	types.ResourceIsolateConfig
	eventChan        chan *types.ResourceUpdateEvent
	podInformer      cache.SharedIndexInformer
	resourceManagers []manager.ResourceQosManager
	onlineInterface  online.Interface
}

var _ Manager = &QosK8sManager{}

// NewQosK8sManager create k8s qos manager
func NewQosK8sManager(config types.ResourceIsolateConfig, stStore statestore.StateStore,
	predict predict.Interface, podInformer cache.SharedIndexInformer, conflict conflict.Manager,
	onlineInterface online.Interface) Manager {
	// register kinds of resource manager
	var resourceManagers []manager.ResourceQosManager
	for _, r := range []string{manager.QosCPU, manager.QosMemory, manager.QosDiskIO, manager.QosNetIO} {
		disable, ok := config.ResourceDisable[r]
		if ok && disable {
			klog.Warningf("Qos module resource %s is disabled", r)
			continue
		}

		var handler func() manager.ResourceQosManager
		switch r {
		case manager.QosCPU:
			handler = func() manager.ResourceQosManager {
				klog.V(2).Infof("cpu isolate policy is: %s", config.CpuConfig.ManagePolicy)
				switch config.CpuConfig.ManagePolicy {
				case types.CpuManagePolicyBT:
					return manager.NewQosCpuBT(config.CpuConfig.KubeletStatic)
				case types.CpuManagePolicySet:
					return manager.NewQosCpuSet(config.CpuConfig.CpuSetConfig)
				case types.CpuManagePolicyQuota:
					return manager.NewQosCpuQuota(config.CpuConfig.KubeletStatic, config.CpuConfig.CpuQuotaConfig)
				case types.CpuManagePolicyAdaptive:
					return manager.NewQosCpuAdaptive(config.CpuConfig.KubeletStatic, config.CpuConfig.CpuSetConfig, stStore)
				}
				return nil
			}
		case manager.QosMemory:
			handler = func() manager.ResourceQosManager {
				return manager.NewQosMemory(stStore)
			}
		case manager.QosDiskIO:
			handler = func() manager.ResourceQosManager {
				return manager.NewQosDiskIO(config.DiskNames)
			}
		case manager.QosNetIO:
			handler = func() manager.ResourceQosManager {
				return manager.NewQosNetIO(config.Iface, config.EniIface)
			}
		}
		if handler != nil {
			resourceManagers = append(resourceManagers, handler())
		}
	}

	return &QosK8sManager{
		ResourceIsolateConfig: config,
		predict:               predict,
		stStore:               stStore,
		conflict:              conflict,
		eventChan:             make(chan *types.ResourceUpdateEvent, 32),
		podInformer:           podInformer,
		resourceManagers:      resourceManagers,
		onlineInterface:       onlineInterface,
	}
}

// Name show module name
func (q *QosK8sManager) Name() string {
	return "ModuleQosK8s"
}

// Run start the main loop and isolate offline resources periodically
func (q *QosK8sManager) Run(stop <-chan struct{}) {
	for _, m := range q.resourceManagers {
		if err := m.PreInit(); err != nil {
			klog.Fatalf("Qos module pre init resource(%s) failed: %v", m.Name(), err)
		}
	}

	go func() {
		// execute just for first time
		q.UpdateResource()

		updateTicker := time.NewTicker(q.UpdatePeriod.TimeDuration())
		defer updateTicker.Stop()
		for {
			select {
			case <-updateTicker.C:
				klog.V(4).Infof("resource update periodically")
				q.UpdateResource()
			case event := <-q.eventChan:
				klog.Infof("receive resource update event: %+v", event)
				q.UpdateResource()
			}
		}
	}()

	for _, m := range q.resourceManagers {
		m.Run(stop)
	}

	if q.OnlineType == types.OnlineTypeOnLocal && len(q.ExternalComponents) > 0 {
		go wait.Until(q.moveSystemComponents, time.Second*15, stop)
	}

	klog.V(2).Info("QOS module starting successfully")
}

// moveSystemComponents move system components to offline cgroup and set oom_score_adj
func (q *QosK8sManager) moveSystemComponents() {
	err := cgroup.EnsureCgroupPath(types.CgroupOfflineSystem)
	if err != nil {
		klog.Errorf("failed ensure cgroup path %v: %v", types.CgroupOfflineSystem, err)
		return
	}
	cgroup.CPUOfflineSet(types.CgroupOfflineSystem, true)
	subSystemsNeedMove := []string{cgroup.MemorySubsystem, cgroup.CPUSubsystem}
	for _, component := range q.ExternalComponents {
		if len(component.Cgroup) > 0 {
			_, err := cgroup.MovePids(subSystemsNeedMove, component.Cgroup, types.CgroupOfflineSystem)
			if err != nil {
				klog.Errorf("failed move pids to %s: %v", types.CgroupOfflineSystem, err)
			}
		}
		if len(component.Command) > 0 {
			// TODO: support move based on command
		}
	}
	memCgPath := cgroup.GetMemoryCgroupPath(types.CgroupOfflineSystem)
	pids, err := cgroup.GetPids(memCgPath)
	if err != nil {
		klog.Errorf("failed get pids from cgroup %v: %v", memCgPath, err)
		return
	}
	for _, pid := range pids {
		pidStr := strconv.Itoa(pid)
		oomScoreAdjPath := path.Join("/proc", pidStr, "oom_score_adj")
		err := ioutil.WriteFile(oomScoreAdjPath, []byte(types.SystemComponentOomScoreAdj), 0700)
		if err != nil {
			klog.Errorf("failed to adjust oom_score_adj for pid %v: %v", pid, err)
		}
	}
}

// UpdateEvent accept event notification to update offline resource timely
func (q *QosK8sManager) UpdateEvent(event *types.ResourceUpdateEvent) error {
	select {
	case q.eventChan <- event:
	default:
		klog.Errorf("qos event chan is full, drop event: %+v", event)
		return fmt.Errorf("qos manager channel is full")
	}

	return nil
}

// UpdateResource calling kinds of resource managers to isolate offline resources
func (q *QosK8sManager) UpdateResource() {
	resCfg := q.getResourceConfig()
	if resCfg == nil {
		klog.Errorf("resource config is nil, ignore QOS checking this time")
		return
	}

	offlineResources := resCfg.Resources
	// There maybe some conflicts on some resource, and should decrease the resource for offline
	conflictedList, err := q.conflict.CheckAndSubConflictResource(offlineResources)
	if err != nil {
		klog.Errorf("sub conflicted resources err: %v", err)
	} else {
		if len(conflictedList) > 0 {
			klog.Infof("after remove conflicted resources(%v): %+v",
				conflictedList, offlineResources)
		}
		klog.V(4).Infof("current resource config removing conflicted resources: %v",
			resCfg)
	}

	// assign minimum resource value
	checkResourceAvailable(offlineResources)
	resCfg.Resources = offlineResources

	if q.OfflineType == types.OfflineTypeOnk8s {
		moveOfflinePidsTogether(resCfg.OfflineCgroups)
	}

	// calling kinds of resource manger to isolate offline jobs
	for _, m := range q.resourceManagers {
		err = m.Manage(resCfg)
		if err != nil {
			klog.Errorf("isolate resource %s err: %v", m.Name(), err)
		}
	}
}

// getResourceConfig gets online/offline cgroups, and offline resource quantity
func (q *QosK8sManager) getResourceConfig() *manager.CgroupResourceConfig {
	// get all pods
	var podList []*v1.Pod
	allPods := q.podInformer.GetStore().List()
	for _, p := range allPods {
		pod := p.(*v1.Pod)
		podList = append(podList, pod)
	}
	cgR := &manager.CgroupResourceConfig{
		Resources: q.predict.GetAllocatableForBatch(),
		PodList:   podList,
	}

	for _, pod := range podList {
		if appclass.IsOffline(pod) {
			cgR.OfflineCgroups = append(cgR.OfflineCgroups, appclass.PodCgroupDirs(pod)...)
			continue
		}
		cgR.OnlineCgroups = append(cgR.OnlineCgroups, appclass.PodCgroupDirs(pod)...)
	}
	if q.onlineInterface != nil {
		onlineCgroups := q.onlineInterface.GetOnlineCgroups()
		cgR.OnlineCgroups = append(cgR.OnlineCgroups, onlineCgroups...)
	}
	klog.V(4).Infof("current resource config: %v", cgR)

	return cgR
}

// moveOfflinePidsTogether move all offline pids to the same cgroup, just for offline type is k8s.
// for yarn on k8s, offline pids have already in the nodemanager container path
// The function is used to collect some common features for all offline jobs,
// such as collecting process which in D state
func moveOfflinePidsTogether(offlineCgs []string) {
	subSystemsNeedMove := []string{cgroup.DevicesSubsystem}
	targetCgroup := types.CgroupOffline
	for _, cg := range offlineCgs {
		klog.V(4).Infof("starting remove pids for %v from %s to %s", subSystemsNeedMove, cg, targetCgroup)
		_, err := cgroup.MovePids(subSystemsNeedMove, cg, targetCgroup)
		if err != nil {
			klog.Errorf("move pid from cgroup %s to %s err: %v", cg, targetCgroup, err)
		}
	}
}

// checkResourceAvailable check minimum value for resources
func checkResourceAvailable(res v1.ResourceList) {
	minCpu := *resource.NewMilliQuantity(1000, resource.DecimalSI)
	minMem := *resource.NewQuantity(2048*types.MemUnit, resource.DecimalSI)

	if cpu, ok := res[v1.ResourceCPU]; ok {
		if cpu.Cmp(minCpu) < 0 {
			klog.Warningf("isolate resource, cpu is too small(%v), setting min value: %v", cpu, minCpu)
			res[v1.ResourceCPU] = minCpu
		}
	}
	if mem, ok := res[v1.ResourceMemory]; ok {
		if mem.Cmp(minMem) < 0 {
			klog.Warningf("isolate resource, memory is too small(%v), setting min value: %v", mem, minMem)
			res[v1.ResourceMemory] = minMem
		}
	}
}
