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

package cgroupstore

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/cadvisor"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	// Register supported container handlers.
	_ "github.com/google/cadvisor/container/docker/install"

	"github.com/google/cadvisor/cache/memory"
	cadvisormetrics "github.com/google/cadvisor/container"
	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	cadvisorutils "github.com/google/cadvisor/utils"
	"github.com/google/cadvisor/utils/sysfs"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

var machineCores = 0
var cgroupStatsExample = &CgroupStats{}
var _ CgroupStore = &cgroupStoreManager{}

type cgroupStoreManager struct {
	cadvisorManager       cadvisor.Cadvisor
	cadvisorParam         cadvisor.CadvisorParameter
	cadvisorLock          sync.RWMutex
	cgroups               []string
	podInformer           cache.SharedIndexInformer
	started               bool
	cacheAge              time.Duration
	cacheLock             sync.RWMutex
	caches                map[string]*cadvisorutils.TimedStore
	cachesCollectInterval time.Duration
}

// NewCgroupStoreManager create a ContainerStats instance to collect container metrics
func NewCgroupStoreManager(config types.MetricsContainerConfig,
	podInformer cache.SharedIndexInformer) CgroupStore {
	var cacheAge = time.Duration(10 * time.Minute)
	includeMetrics := cadvisormetrics.MetricSet{}
	for _, r := range config.Resources {
		includeMetrics[cadvisormetrics.MetricKind(r)] = struct{}{}
	}

	cadPram := cadvisor.CadvisorParameter{
		MemCache:                memory.New(cacheAge, nil),
		SysFs:                   sysfs.NewRealSysFs(),
		IncludeMetrics:          includeMetrics,
		MaxHousekeepingInterval: config.MaxHousekeepingInterval.TimeDuration(),
	}

	for _, cg := range config.Cgroups {
		err := cgroup.EnsureCgroupPath(cg)
		if err != nil {
			klog.Fatalf("ensure cgroup path(%s) failed: %v", cg, err)
		}
	}

	cgStore := &cgroupStoreManager{
		cadvisorManager:       cadvisor.NewCadvisorManager(cadPram, config.Cgroups),
		cadvisorParam:         cadPram,
		cgroups:               config.Cgroups,
		podInformer:           podInformer,
		caches:                make(map[string]*cadvisorutils.TimedStore),
		cacheAge:              cacheAge,
		cachesCollectInterval: config.CollectInterval.TimeDuration(),
	}
	return cgStore
}

// MachineInfo get machine metrics
func (c *cgroupStoreManager) MachineInfo() (*cadvisorapi.MachineInfo, error) {
	return c.cadvisorManager.MachineInfo()
}

// ListAllCgroups list all cgroups
func (c *cgroupStoreManager) ListAllCgroups(classFilter sets.String) (map[string]*CgroupRef, error) {
	var maxAge *time.Duration
	cgs := make(map[string]*CgroupRef)

	c.cadvisorLock.RLock()
	defer c.cadvisorLock.RUnlock()
	for _, cg := range c.cgroups {
		infoMap, err := c.cadvisorManager.ContainerInfoV2(cg, cadvisorapiv2.RequestOptions{
			IdType:    cadvisorapiv2.TypeName,
			Count:     1,
			Recursive: true,
			MaxAge:    maxAge,
		})
		if err != nil {
			klog.Errorf("get container info for cgroup %s err:%v", cg, err)
			continue
		}

		for k, v := range infoMap {
			ref, _, err := c.generateRef(k, v)
			if err != nil {
				klog.V(5).Infof("cgroup path(%s) not generate ref: %v", k, err)
				continue
			}

			if classFilter.Len() != 0 && !classFilter.Has(string(ref.AppClass)) {
				klog.V(5).Infof("cgroup path(%s) is filtered: %s", k, ref.AppClass)
				continue
			}

			cgs[k] = ref
		}
	}

	return cgs, nil
}

// GetCgroupResourceRangeStats get cgroup resource usage list by pod name and timestamp
func (c *cgroupStoreManager) GetCgroupResourceRangeStats(podName, podNamespace string,
	start, end time.Time, count int) ([]*CgroupStats, error) {
	var targetCache *cadvisorutils.TimedStore
	var cSts []*CgroupStats

	func() {
		c.cadvisorLock.RLock()
		defer c.cadvisorLock.RUnlock()

		for _, cgCache := range c.caches {
			item := cgCache.Get(0).(*CgroupStats)
			if item.Ref.PodName == podName && item.Ref.PodNamespace == podNamespace {
				targetCache = cgCache
				break
			}
		}
	}()
	if targetCache == nil {
		return cSts, fmt.Errorf("cgroup resource stats not found the pod %s-%s", podNamespace, podName)
	}

	items := targetCache.InTimeRange(start, end, count)
	for _, i := range items {
		cSts = append(cSts, i.(*CgroupStats))
	}

	return cSts, nil
}

// GetCgroupResourceRecentState get cgroup resource usage state by container name
func (c *cgroupStoreManager) GetCgroupResourceRecentState(podName, podNamespace string,
	updateStats bool) (*CgroupStats, error) {
	c.cadvisorLock.RLock()
	defer c.cadvisorLock.RUnlock()

	cgs, err := c.listCgroupStats(updateStats, true, c.cgroups, &CgroupRef{
		PodName:      podName,
		PodNamespace: podNamespace,
	}, sets.NewString())
	if err != nil {
		return nil, err
	}

	if len(cgs) > 1 {
		return nil, fmt.Errorf("more cgroup stats found for container(%s-%s): %+v",
			podNamespace, podName, cgs)
	}

	for _, cg := range cgs {
		return cg, nil
	}

	return nil, fmt.Errorf("no cgroup stats found for container(%s-%s): %+v",
		podNamespace, podName, cgs)
}

// GetCgroupResourceRecentStateByPath get cgroup resource usage state by cgroup path
func (c *cgroupStoreManager) GetCgroupResourceRecentStateByPath(cgPath string,
	updateStats bool) (*CgroupStats, error) {
	cgs, err := c.listCgroupStats(updateStats, false, []string{cgPath}, nil, sets.NewString())
	if err != nil {
		return nil, err
	}

	if len(cgs) > 1 {
		return nil, fmt.Errorf("more cgroup stats found for path: %s", cgPath)
	}

	for _, cg := range cgs {
		return cg, nil
	}

	return nil, fmt.Errorf("no cgroup stats found for path: %s", cgPath)
}

// ListCgroupResourceRangeStats list all cgroup data in assigned period time
func (c *cgroupStoreManager) ListCgroupResourceRangeStats(start, end time.Time, count int,
	classFilter sets.String) (map[string][]*CgroupStats, error) {
	totalCgStats := make(map[string][]*CgroupStats)

	c.cacheLock.RLock()
	defer c.cacheLock.RUnlock()
	for k, cgCache := range c.caches {
		if classFilter.Len() != 0 {
			item := cgCache.Get(0).(*CgroupStats)
			if !classFilter.Has(string(item.Ref.AppClass)) {
				continue
			}
		}

		var cgStats []*CgroupStats
		items := cgCache.InTimeRange(start, end, count)
		for _, i := range items {
			cgStats = append(cgStats, i.(*CgroupStats))
		}
		totalCgStats[k] = cgStats
	}

	return totalCgStats, nil
}

// ListCgroupResourceRecentState list available cgroups and calculate resource usage
func (c *cgroupStoreManager) ListCgroupResourceRecentState(updateStats bool,
	classFilter sets.String) (map[string]*CgroupStats, error) {
	c.cadvisorLock.RLock()
	defer c.cadvisorLock.RUnlock()
	return c.listCgroupStats(updateStats, true, c.cgroups, nil, classFilter)
}

// Run main loop
func (c *cgroupStoreManager) Run(stop <-chan struct{}) {
	func() {
		klog.V(2).Info("start cadvisor to collect cgroup resources")
		c.cadvisorLock.Lock()
		defer c.cadvisorLock.Unlock()

		err := c.cadvisorManager.Start()
		if err != nil {
			klog.Fatalf("cadvisor manager start err: %v", err)
		}
		c.started = true
	}()

	go wait.Until(c.collectCgroupStats, c.cachesCollectInterval, stop)
	return
}

// GetCgroupStoreSupportedTags get cgroup state supported resource tags
func (c *cgroupStoreManager) GetCgroupStoreSupportedTags() []string {
	return cgroupStatsExample.GetTags()
}

func (c *cgroupStoreManager) collectCgroupStats() {
	var err error
	var totalCgStats map[string]*CgroupStats

	func() {
		c.cadvisorLock.RLock()
		defer c.cadvisorLock.RUnlock()
		totalCgStats, err = c.listCgroupStats(false, true, c.cgroups, nil, sets.NewString())
	}()
	if err != nil {
		klog.Errorf("list total cgroup stats err: %v", err)
		return
	}

	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	for k, cgStats := range totalCgStats {
		cache, ok := c.caches[k]
		if !ok {
			cache = cadvisorutils.NewTimedStore(c.cacheAge, -1)
			c.caches[k] = cache
		}
		cache.Add(cgStats.Timestamp, cgStats)
	}

	// delete caches which is not updated for long time
	for k, cache := range c.caches {
		if len(cache.InTimeRange(time.Now().Add(-c.cacheAge), time.Now(), -1)) == 0 {
			delete(c.caches, k)
		}
	}
}

func (c *cgroupStoreManager) listCgroupStats(updateStats, recur bool, cgPaths []string,
	cgKey *CgroupRef, classFilter sets.String) (map[string]*CgroupStats, error) {
	cgs := make(map[string]*CgroupStats)

	var maxAge *time.Duration
	if updateStats {
		age := 0 * time.Second
		maxAge = &age
	}

	for _, cg := range cgPaths {
		infoMap, err := c.cadvisorManager.ContainerInfoV2(cg, cadvisorapiv2.RequestOptions{
			IdType:    cadvisorapiv2.TypeName,
			Count:     2,
			Recursive: recur,
			MaxAge:    maxAge,
		})
		if err != nil {
			klog.Errorf("get container info for cgroup %s err:%v", cg, err)
			continue
		}

		for k, v := range infoMap {
			if len(v.Stats) != 2 {
				continue
			}
			if !v.Spec.HasCpu && !v.Spec.HasMemory {
				continue
			}

			// generate key
			ref, key, err := c.generateRef(k, v)
			if err != nil {
				klog.V(5).Infof("cgroup path(%s) not generate ref: %v", k, err)
				continue
			}
			// if cgKey not nil, then just output cgroup stats for the cgKey
			if cgKey != nil {
				if cgKey.PodName != ref.PodName ||
					cgKey.PodNamespace != ref.PodNamespace {
					continue
				}
			}
			if classFilter.Len() != 0 && !classFilter.Has(string(ref.AppClass)) {
				continue
			}

			cgStat := c.generateCgroupStats(&v, ref)
			if cg, ok := cgs[key]; ok {
				cgStat.CpuUsage += cg.CpuUsage
				cgStat.NrCpuThrottled += cg.NrCpuThrottled
				cgStat.MemoryTotalUsage += cg.MemoryTotalUsage
				cgStat.MemoryCacheUsage += cg.MemoryCacheUsage
				cgStat.MemoryWorkingSetUsage += cg.MemoryWorkingSetUsage
				cgStat.TcpOOM += cg.TcpOOM
				cgStat.ListenOverflows += cg.ListenOverflows
				cgStat.RxBps += cg.RxBps
				cgStat.TxBps += cg.TxBps
				cgStat.RxPps += cg.RxPps
				cgStat.TxPps += cg.TxPps
			}
			cgs[key] = cgStat
		}
	}
	return cgs, nil
}

func (c *cgroupStoreManager) generateCgroupStats(v *cadvisorapiv2.ContainerInfo, ref *CgroupRef) *CgroupStats {
	cgSt := &CgroupStats{
		Ref:       ref,
		Timestamp: v.Stats[1].Timestamp,
	}
	if machineCores == 0 {
		machineInfo, err := c.cadvisorManager.MachineInfo()
		if err == nil && machineInfo != nil {
			machineCores = machineInfo.NumCores
		}
	}

	generateCgroupStatsCpu(v, cgSt)
	generateCgroupStatsMemory(v, cgSt)
	generateCgroupStatsNetwork(v, cgSt)
	return cgSt
}

func generateCgroupStatsCpu(v *cadvisorapiv2.ContainerInfo, cgSt *CgroupStats) {
	if !v.Spec.HasCpu {
		return
	}
	if v.Stats[1].Cpu.Usage.Total >= v.Stats[0].Cpu.Usage.Total {
		total := v.Stats[1].Cpu.Usage.Total - v.Stats[0].Cpu.Usage.Total
		interval := v.Stats[1].Timestamp.Sub(v.Stats[0].Timestamp)
		cgSt.CpuUsage = float64(int(float64(total)*100/float64(interval.Nanoseconds())+0.5)) / 100
		if cgSt.CpuUsage < 0 {
			cgSt.CpuUsage = 0
			klog.Warningf("invalid value when calculating cpu usage, total: %f, %d",
				total, interval.Nanoseconds())
			return
		}
		if machineCores != 0 && cgSt.CpuUsage > float64(machineCores) {
			cgSt.CpuUsage = float64(machineCores)
			klog.Warningf("value too big when calculating cpu usage, just set total cores(%d), total: %f, %d",
				total, interval.Nanoseconds(), machineCores)
			return
		}
		nrThrottled := v.Stats[1].Cpu.CFS.ThrottledPeriods - v.Stats[0].Cpu.CFS.ThrottledPeriods
		nrPeriods := v.Stats[1].Cpu.CFS.Periods - v.Stats[0].Cpu.CFS.Periods
		if nrPeriods > 0 {
			cgSt.NrCpuThrottled = float64(nrThrottled) / float64(nrPeriods)
		}
		cgSt.LoadAvg = float64(v.Stats[1].Cpu.LoadAverage) / 1000
	} else {
		klog.Warningf("cumulative cpu stats decrease")
	}
}

func generateCgroupStatsMemory(v *cadvisorapiv2.ContainerInfo, cgSt *CgroupStats) {
	if !v.Spec.HasMemory {
		return
	}
	cgSt.MemoryTotalUsage = float64(v.Stats[1].Memory.Usage)
	cgSt.MemoryWorkingSetUsage = float64(v.Stats[1].Memory.WorkingSet)
	cgSt.MemoryCacheUsage = float64(v.Stats[1].Memory.Cache)
}

func generateCgroupStatsNetwork(v *cadvisorapiv2.ContainerInfo, cgSt *CgroupStats) {
	if !v.Spec.HasNetwork {
		return
	}
	interval := v.Stats[1].Timestamp.Sub(v.Stats[0].Timestamp).Seconds()
	cgSt.TcpOOM = float64(v.Stats[1].Network.TcpAdvanced.TCPAbortOnMemory-
		v.Stats[0].Network.TcpAdvanced.TCPAbortOnMemory) / interval
	cgSt.ListenOverflows = float64(v.Stats[1].Network.TcpAdvanced.ListenOverflows-
		v.Stats[0].Network.TcpAdvanced.ListenOverflows) / interval
	var rxPkts, txPkts [2]uint64
	var rxBytes, txBytes [2]uint64
	for i := 0; i < 2; i++ {
		for _, iface := range v.Stats[i].Network.Interfaces {
			rxPkts[i] += iface.RxPackets
			txPkts[i] += iface.TxPackets
			rxBytes[i] += iface.RxBytes
			txBytes[i] += iface.TxBytes
		}
	}
	cgSt.RxPps = float64(rxPkts[1]-rxPkts[0]) / interval
	cgSt.TxPps = float64(txPkts[1]-txPkts[0]) / interval
	cgSt.RxBps = float64(rxBytes[1]-rxBytes[0]) / interval
	cgSt.TxBps = float64(txBytes[1]-txBytes[0]) / interval
}

// generateRef generate pod info, and also return err if the cgroup path is invalid
func (c *cgroupStoreManager) generateRef(cgPath string,
	con cadvisorapiv2.ContainerInfo) (*CgroupRef, string, error) {
	ref := &CgroupRef{
		Name:          cgPath,
		ContainerName: GetContainerName(con.Spec.Labels),
		PodName:       GetPodName(con.Spec.Labels),
		PodNamespace:  GetPodNamespace(con.Spec.Labels),
		PodUid:        GetPodUID(con.Spec.Labels),
	}

	// generate key identification and container name for this cgroup
	key := getCgroupId(ref)

	if err := checkCgroupAvailable(cgPath, ref); err != nil {
		return nil, "", fmt.Errorf("cgroup path(%s) is invalid: %v", cgPath, err)
	}

	// set online flag
	// cgroups like /kubepods or /kubepods/burstable, they are not online cgroup.
	// While we need to collect resource from these cgroups.
	c.assignAppClassFlag(cgPath, ref)

	return ref, key, nil
}

func getCgroupId(ref *CgroupRef) string {
	if len(ref.PodUid) != 0 {
		// the cgroup is created by k8s
		return generateK8sCgroupId(ref.PodUid, ref.ContainerName)
	}

	// the cgroup is just common path, may be created manually
	// there is problem when two cgroup has the same name with different cgroup path
	id := ""
	index := strings.LastIndex(ref.Name, "/")
	if index == -1 {
		id = ref.Name
	} else {
		id = ref.Name[index+1:]
	}

	// the containerName is nil, we will assign the id name.
	ref.ContainerName = id

	return id
}

func generateK8sCgroupId(podUid, containerName string) string {
	return podUid[:5] + "_" + containerName
}

func (c *cgroupStoreManager) assignAppClassFlag(cgPath string, ref *CgroupRef) {
	if cgPath == types.CgroupOfflineSystem {
		ref.AppClass = appclass.AppClassOffline
		return
	}
	if ref.PodName != "" && ref.PodNamespace != "" {
		obj, exists, err := c.podInformer.GetStore().Get(&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ref.PodName,
				Namespace: ref.PodNamespace,
			}})
		if err == nil && exists {
			ref.AppClass = appclass.GetAppClass(obj.(*v1.Pod))
		} else {
			ref.AppClass = appclass.AppClassUnknown
		}
		return
	}
	// for non-k8s online tasks
	if c.isNonK8sOnline(cgPath) {
		ref.AppClass = appclass.AppClassOnline
	} else {
		ref.AppClass = appclass.AppClassAggregatedCgroup
	}
}

// check if the job, which not running on k8s, is online type
func (c *cgroupStoreManager) isNonK8sOnline(cgPath string) bool {
	switch cgPath {
	case types.CgroupNonK8sOnline, types.CgroupKubePods:
		return false
	default:
		if strings.HasPrefix(cgPath, types.CgroupNonK8sOnline) {
			return true
		}
		for _, cg := range c.cgroups {
			if cgPath == cg {
				return true
			}
		}
	}
	return false
}

func checkCgroupAvailable(cgPath string, ref *CgroupRef) error {
	if cgPath == types.CgroupOfflineSystem {
		return nil
	}
	// ignored cgroup path like:
	// /kubepods/burstable/pod0ab20a34-2209-4781-be9c-983f83969f74/
	// 2d1d90dbd8b14d0283e3747181c89f7c65e106c7f83c49405fb9a56f0b360197/system.slice/crond.service/
	subs := strings.Split(strings.Trim(cgPath, "/"), "/")
	if len(subs) > 4 {
		// kubepods/offline/besteffort/pod2a575fe0-a445-4998-9145-eaafec13b973/
		// a8b996f48a4d3080048eda77febb6b3a4a65de245b89edaba970444b6aafdf49/hadoop-yarn
		yarnPath := false
		if strings.HasPrefix(cgPath, types.CgroupOffline) {
			if len(subs) == 5 || (len(subs) == 6 && subs[5] == types.CgroupYarn) {
				// special for nodemanager
				yarnPath = true
			}
		}

		if !yarnPath {
			return fmt.Errorf("cgroup path too long")
		}
	}

	if ref.ContainerName == PodInfraContainerName {
		return fmt.Errorf("cgroup path is infra container")
	}

	// ignore poduid path, such as:pod0b33fbdd-7c5a-43cd-80e7-bdde8791b154
	if len(ref.ContainerName) == 39 && strings.HasPrefix(ref.ContainerName, "pod") {
		return fmt.Errorf("cgroup path is podxxx")
	}

	return nil
}

// AddExtraCgroups add new cgroups, which is not managed by k8s
func (c *cgroupStoreManager) AddExtraCgroups(extraCgs []string) error {
	if len(extraCgs) == 0 {
		return nil
	}
	klog.V(2).Infof("receiving new extra cgroups: %+v", extraCgs)

	c.cadvisorLock.Lock()
	defer c.cadvisorLock.Unlock()

	hasNew := false
	existingCgs := sets.NewString(c.cgroups...)
	for _, cg := range extraCgs {
		if !existingCgs.Has(cg) {
			c.cgroups = append(c.cgroups, cg)
			hasNew = true
			// record new cgroup, in case the repeat key
			existingCgs.Insert(cg)
		}
	}
	if !hasNew {
		klog.V(3).Infof("extra cgroups have already existed: %+v", extraCgs)
		return nil
	}
	klog.V(2).Infof("current cgroups: %+v", c.cgroups)

	if !c.started {
		klog.V(2).Infof("cadvisor manager has not started, just replacing manager")
		c.cadvisorManager = cadvisor.NewCadvisorManager(c.cadvisorParam, c.cgroups)
	} else {
		klog.V(2).Infof("cadvisor manager has started, need restart")
		return c.restartCadvisor()
	}

	return nil
}

func (c *cgroupStoreManager) restartCadvisor() error {
	err := c.cadvisorManager.Stop()
	if err != nil {
		klog.Errorf("cadvisor manager stop err: %v", err)
		return err
	}

	c.cadvisorManager = cadvisor.NewCadvisorManager(c.cadvisorParam, c.cgroups)
	err = c.cadvisorManager.Start()
	if err != nil {
		klog.Errorf("cadvisor manager restart err: %v", err)
	} else {
		klog.V(2).Infof("cadvisor manager restart successfully")
	}

	return err
}
