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

package resource

import (
	"fmt"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/alarm"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/predict"
	k8sres "github.com/tencent/caelus/pkg/caelus/resource/k8s"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apires "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

var _ clientInterface = (*k8sClient)(nil)

// OfflineK8sData describe options needed for k8s offline type
type OfflineK8sData struct {
	OfflineOnK8sCommonData
	// node informer
	NodeInformer cache.SharedIndexInformer
}

// K8sMetadata describe meta data for k8s type
type K8sMetadata struct {
	PodName   string
	Namespace string
}

// k8sClient group node resource manager for k8s
type k8sClient struct {
	node     *v1.Node
	lastNode *v1.Node
	types.NodeResourceConfig
	OfflineK8sData
	predictor predict.Interface
	// nodeInfo store pods and total requested resources on the node
	nodeInfo               *k8sres.NodeInfo
	extResScheduleDisabled bool
	extResScheduleLock     sync.RWMutex
	conflict               conflict.Manager
}

// newK8sResourceManager new an instance for managing k8s node capacity
func newK8sClient(config types.NodeResourceConfig, predictor predict.Interface,
	conflict conflict.Manager, offlineData interface{}) clientInterface {
	k8sData := offlineData.(*OfflineK8sData)
	kc := &k8sClient{
		NodeResourceConfig:     config,
		OfflineK8sData:         *k8sData,
		predictor:              predictor,
		nodeInfo:               nil,
		extResScheduleDisabled: false,
		conflict:               conflict,
	}

	kc.node = &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.NodeName(),
		},
	}

	return kc
}

// Init waiting until resource cache is ready
func (k *k8sClient) Init() error {
	return nil
}

// CheckPoint check schedule state
func (k *k8sClient) CheckPoint() error {
	metrics.NodeScheduleDisabled(0)
	// for k8s, disable schedule action is done by native agent, and could check time available
	checkScheduleDisable(true, k.EnableOfflineSchedule, k.DisableOfflineSchedule)

	return nil
}

// module name
func (k *k8sClient) Name() string {
	return "ModuleResourceK8s"
}

// Run do nothing
func (k *k8sClient) Run(stopCh <-chan struct{}) {
}

// AdaptAndUpdateOfflineResource adapt and update offline resource list based on different conditions
func (k *k8sClient) AdaptAndUpdateOfflineResource(offlineList v1.ResourceList, conflictingResources []string) error {
	obj, exited, err := k.NodeInformer.GetStore().Get(k.node)
	if err != nil {
		klog.Errorf("k8s get node info err: %v", err)
		return err
	}
	if !exited {
		klog.Errorf("node not found: %v", k.node)
		return fmt.Errorf("node not found")
	}
	// re-assign new node info
	k.node = obj.(*v1.Node)

	// refresh current pods on the node
	k.syncNodeInfo(k.node)
	klog.V(4).Infof("all offline requested resource: %+v", k.nodeInfo.RequestedResource)

	var chosenPod *v1.Pod
	// check if need to kill pods, and do the killing action
	func() {
		over, rs := k.nodeInfo.More(offlineList)

		if len(conflictingResources) == 0 {
			if k.DisableKillIfNormal {
				if over {
					klog.Warning("current resources are overhead without conflicting, no need to kill pod")
				}
				return
			}
		} else {
			if k.OnlyKillIfIncompressibleRes && types.AllResCompressible(conflictingResources) {
				klog.Warningf("conflicting resources(%v) are compressible, no need to kill pod",
					conflictingResources)
				return
			}
		}

		// kill pod if the current resource is overhead, or there are conflicting resources
		if over {
			klog.Infof("current resource is overhead, should kill pods")
			chosenPod = k.reduceReleaseRequestResource(rs)
		} else if len(conflictingResources) != 0 {
			klog.Infof("there are conflicting resources, should kill at least one pod")
			chosenPod = k.reduceReleaseRequestResource(v1.ResourceName(conflictingResources[0]))
		}
	}()
	metrics.NodeResourceMetricsReset(k.nodeInfo.RequestedResource, metrics.NodeResourceTypeOfflineAllocated)

	// check if need to disable schedule for extended resource
	// disable schedule by making quantity of allocatable cpu resource equal to quantity of all requested cpu resource
	scheduleDisabled := false
	func() {
		k.extResScheduleLock.RLock()
		defer k.extResScheduleLock.RUnlock()
		if k.extResScheduleDisabled {
			scheduleDisabled = true
		}
	}()
	if scheduleDisabled {
		klog.V(4).Infof("current all requested resources: %+v", k.nodeInfo.RequestedResource)
		if k.nodeInfo.Less(offlineList) {
			// should not set the value as zero, for running pods wil be evicted when kubelet restarted
			offlineList[v1.ResourceCPU] = k.nodeInfo.RequestedResource[v1.ResourceCPU]
		}
		klog.Infof("sync node resources, after disable offline schedule resources: %+v", offlineList)
	}
	metrics.NodeResourceMetricsReset(offlineList, metrics.NodeResourceTypeOfflineFormat)

	err = k.syncOfflineResource(offlineList)
	if err != nil {
		return err
	}

	// kill action should be done after syncing offline resources, for the pod may be scheduled on the node between the
	// kill time and capacity update time, which may lead to "out of xxx"
	if chosenPod != nil {
		// waiting for the scheduler to receive the event, in case of "out of xxx"
		time.Sleep(1 * time.Second)
		err = k.killPod(chosenPod)
		if err != nil {
			klog.Errorf("kill pod err: %v", err)
		}
	}

	return nil
}

// syncOfflineResource update node capacity
func (k *k8sClient) syncOfflineResource(offlineList v1.ResourceList) error {
	var err error
	metrics.NodeResourceMetricsReset(offlineList, metrics.NodeResourceTypeOfflineCapacity)

	updatedNode := k.updateNodeResources(k.node, offlineList)
	if k.lastNode != nil &&
		apiequality.Semantic.DeepEqual(k.lastNode.Status.Capacity, updatedNode.Status.Capacity) {
		klog.V(4).Infof("Node %s resource is not changed, ignore", updatedNode.Name)
		return nil
	}

	for i := 0; i < 3; i++ {
		_, err := k.Client.CoreV1().Nodes().UpdateStatus(updatedNode)
		if err == nil {
			break
		}

		if errors.IsConflict(err) {
			continue
		}
	}

	if err == nil {
		k.lastNode = updatedNode
		klog.V(2).Infof("Update node %s status", updatedNode.Name)
	} else {
		klog.Errorf("can't update node %s, %v", updatedNode.Name, err)
	}

	return err
}

// DisableOfflineSchedule mark schedule disabled state
func (k *k8sClient) DisableOfflineSchedule() error {
	k.extResScheduleLock.Lock()
	defer k.extResScheduleLock.Unlock()

	if k.extResScheduleDisabled {
		klog.V(4).Infof("schedule is already closed")
		return nil
	}
	alarm.SendAlarm("schedule is closing")
	klog.V(2).Infof("schedule is closing")
	k.extResScheduleDisabled = true
	metrics.NodeScheduleDisabled(1)
	storeCheckpoint(true)

	return nil
}

// EnableOfflineSchedule recover schedule enabled state
func (k *k8sClient) EnableOfflineSchedule() error {
	k.extResScheduleLock.Lock()
	defer k.extResScheduleLock.Unlock()

	if !k.extResScheduleDisabled {
		return nil
	}
	alarm.SendAlarm("schedule is opening")
	klog.V(2).Infof("schedule is opening")
	k.extResScheduleDisabled = false
	metrics.NodeScheduleDisabled(0)
	storeCheckpoint(false)

	return nil
}

// OfflineScheduleDisabled return true if schedule disabled for offline jobs
func (k *k8sClient) OfflineScheduleDisabled() bool {
	k.extResScheduleLock.Lock()
	defer k.extResScheduleLock.Unlock()
	return k.extResScheduleDisabled
}

// GetOfflineJobs return current offline job list
func (k *k8sClient) GetOfflineJobs() ([]types.OfflineJobs, error) {
	offlineJobs := []types.OfflineJobs{}
	_, offlinePods := k.getPodList()
	for _, pod := range offlinePods {
		metadata := &K8sMetadata{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
		}
		offlineJobs = append(offlineJobs, types.OfflineJobs{
			Metadata: metadata,
		})
	}

	return offlineJobs, nil
}

// KillOfflineJob kill k8s offline pod based on conflicting resource
func (k *k8sClient) KillOfflineJob(conflictingResource v1.ResourceName) {
	chosenPod, _, err := k.chooseOfflinePod(conflictingResource)
	if err != nil {
		klog.Errorf("choose offline pod to kill failed: %v", err)
		return
	}
	if chosenPod == nil {
		klog.Warningf("choose offline pod to kill, got nil")
		return
	}

	err = k.killPod(chosenPod)
	if err != nil {
		klog.Errorf("kill offline pod failed: %v", err)
	}
}

// ppdateNodeResources update node resource
func (k *k8sClient) updateNodeResources(node *v1.Node, res v1.ResourceList) *v1.Node {
	updatedNode := node.DeepCopy()

	for k, v := range res {
		resName := v1.ResourceName(fmt.Sprintf("%s%s", k8sres.ExternalResourcePrefix, k))
		klog.V(2).Infof("Set %s to %v", resName, v)
		if k == v1.ResourceCPU {
			vv := apires.NewQuantity(v.MilliValue(), apires.DecimalSI)
			updatedNode.Status.Capacity[resName] = *vv
			updatedNode.Status.Allocatable[resName] = *vv
		} else {
			updatedNode.Status.Capacity[resName] = v
			updatedNode.Status.Allocatable[resName] = v
		}
	}

	return updatedNode
}

func (k *k8sClient) getPodList() (allPods []*v1.Pod, offlinePods []*v1.Pod) {
	objects := k.PodInformer.GetStore().List()
	for _, p := range objects {
		pod := p.(*v1.Pod)
		if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodPending {
			allPods = append(allPods, pod)
			if appclass.IsOffline(pod) {
				offlinePods = append(offlinePods, pod)
			}
		}
	}

	return allPods, offlinePods
}

// syncNodeInfo record current pods on the node
func (k *k8sClient) syncNodeInfo(node *v1.Node) {
	pods, _ := k.getPodList()
	if k.nodeInfo == nil {
		k.nodeInfo = k8sres.NewNodeInfo(pods...)
	} else {
		k.nodeInfo.ResetNodeInfo(pods...)
	}
}

// reduceReleaseRequestResource check how many resources will be released when killing pod
func (k *k8sClient) reduceReleaseRequestResource(rs v1.ResourceName) *v1.Pod {
	chosenPod, releasedRes, err := k.chooseOfflinePod(rs)
	if err != nil {
		klog.Errorf("choose pod err: %v", err)
		return nil
	}
	klog.Infof("resources will release when offline killed: %+v", releasedRes)
	// update all offline requestedResource
	k.nodeInfo.ReduceRequestedResource(releasedRes)
	klog.Infof("update all offline requested resources: %v", k.nodeInfo.RequestedResource)

	return chosenPod
}

// chooseOfflinePod will choose the right pod to kill, and return how many resource will be released
func (k *k8sClient) chooseOfflinePod(rName v1.ResourceName) (*v1.Pod, v1.ResourceList, error) {
	var sortedPods []*v1.Pod
	// pod level resource usage
	usageMap := make(map[string]int64)

	_, offlinePods := k.getPodList()
	for _, p := range offlinePods {
		newPod := p.DeepCopy()
		sortedPods = append(sortedPods, newPod)

		// collecting pod level resource usage
		if len(rName) != 0 {
			resList := k.getPodUsage(newPod)
			pKey := k8sres.GetPodKey(newPod)
			if q, ok := resList[rName]; !ok {
				klog.Errorf("invalid resource name when kill pod: %s", rName)
			} else {
				usageMap[pKey] = q.MilliValue()
			}
		}
	}

	if len(sortedPods) == 0 {
		klog.V(2).Infof("no offline pod found, ignore killing")
		return nil, v1.ResourceList{}, nil
	}

	k8sres.OrderedBy(k8sres.SortByPriority,
		k8sres.SortByResource(usageMap),
		k8sres.SortByStartTime).Sort(sortedPods)
	if klog.V(4) {
		klog.Infof("sorted pods:")
		for _, p := range sortedPods {
			klog.Infof("namespace: %s, name: %s", p.Namespace, p.Name)
		}
	}

	killPod := sortedPods[0]
	releaseResource := k8sres.GetPodResourceRequest(killPod)
	return sortedPods[0], releaseResource, nil
}

// killPod do the evict action
func (k *k8sClient) killPod(chosenPod *v1.Pod) error {
	klog.Infof("start to kill pod: %s-%s", chosenPod.Namespace, chosenPod.Name)
	err := k.Client.CoreV1().Pods(chosenPod.Namespace).Evict(&policy.Eviction{
		ObjectMeta:    chosenPod.ObjectMeta,
		DeleteOptions: metav1.NewDeleteOptions(0),
	})
	if err == nil {
		metrics.KillCounterInc("k8s")
		return nil
	}

	if apierrors.IsTooManyRequests(err) {
		return fmt.Errorf("kill pod(%s-%s) failed: disruption budget deny",
			chosenPod.Namespace, chosenPod.Name)
	}

	return fmt.Errorf("kill pod(%s-%s) failed: %v",
		chosenPod.Namespace, chosenPod.Name, err)
}

// Describe implement prometheus interface
func (k *k8sClient) Describe(ch chan<- *prometheus.Desc) {
}

// Collect implement prometheus interface
func (k *k8sClient) Collect(ch chan<- prometheus.Metric) {
}

func (k *k8sClient) getPodUsage(pod *v1.Pod) v1.ResourceList {
	cpu := apires.NewMilliQuantity(0, apires.DecimalSI)
	mem := apires.NewQuantity(0, apires.DecimalSI)

	cgPaths := appclass.PodCgroupDirs(pod)
	if len(cgPaths) == 0 {
		klog.Errorf("pod(%s-%s) cgroup path not found", pod.Namespace, pod.Name)
	}

	for _, cgPath := range cgPaths {
		cgStat, err := k.StStore.GetCgroupResourceRecentStateByPath(cgPath, true)
		if err != nil {
			klog.Errorf("get cgroup for %s state err: %v", cgPath, err)
		} else {
			cpuUsage := int64(cgStat.CpuUsage * float64(types.CpuUnit))
			cpu.Add(*apires.NewMilliQuantity(cpuUsage, apires.DecimalSI))
			mem.Add(*apires.NewQuantity(int64(cgStat.MemoryTotalUsage), apires.DecimalSI))
		}
	}

	return v1.ResourceList{
		v1.ResourceCPU:    *cpu,
		v1.ResourceMemory: *mem,
	}
}
