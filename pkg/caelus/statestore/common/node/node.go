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

package nodestore

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
	"github.com/tencent/caelus/pkg/caelus/util/machine"

	cadvisor "github.com/google/cadvisor/utils"
	"github.com/google/cadvisor/utils/cpuload/netlink"
	"github.com/guillermo/go.procmeminfo"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/net"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

const (
	nodeResourceName = "Node"
)

var (
	lastNodeResourceState NodeResourceState

	// no need lock
	offlinePodUid  string
	offlineConPath string
	// network devices
	ifaces sets.String
	// cpu total number
	cpuNum float64
	// netlink reader
	netlinkReader *netlink.NetlinkReader
)

var nodeManager *nodeStoreManager

// NodeStoreInterface describe special interface for node resource
type NodeStoreInterface interface {
	GetNodeResourceRangeStats(start, end time.Time, count int) ([]*NodeResourceState, error)
	GetNodeResourceRecentState() (*NodeResourceState, error)
	GetNodeStoreSupportedTags() []string
}

// NodeStore describe common interface for node resource inherit from state store
type NodeStore interface {
	NodeStoreInterface
	Name() string
	Run(stop <-chan struct{})
}

type collector struct {
	name   string
	handle func()
}

type nodeStoreManager struct {
	cacheAge time.Duration
	*types.MetricsNodeConfig
	lock        sync.RWMutex
	caches      map[types.MetricKind]*cadvisor.TimedStore
	podInformer cache.SharedIndexInformer

	collectors []collector
}

// NewNodeStoreManager new node resource state instance
func NewNodeStoreManager(cacheAge time.Duration, config *types.MetricsNodeConfig,
	podInformer cache.SharedIndexInformer) NodeStore {
	// changing network devices to map struct
	ifaces = sets.NewString(config.Ifaces...)

	nodeManager = &nodeStoreManager{
		cacheAge:          cacheAge,
		MetricsNodeConfig: config,
		podInformer:       podInformer,
		caches:            make(map[types.MetricKind]*cadvisor.TimedStore),
		collectors: []collector{
			{
				name:   "TotalNode",
				handle: collectNodeResource,
			},
		},
	}

	return nodeManager
}

// Name describe node resource module name
func (ns *nodeStoreManager) Name() string {
	return nodeResourceName
}

// GetNodeResourceRangeStats list node resource state data in period time
func (ns *nodeStoreManager) GetNodeResourceRangeStats(start, end time.Time, count int) ([]*NodeResourceState, error) {
	var cache *cadvisor.TimedStore
	var nodeSts []*NodeResourceState

	err := func() error {
		var ok bool
		ns.lock.RLock()
		defer ns.lock.RUnlock()

		cache, ok = ns.caches[nodeResourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", nodeResourceName)
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	sts := cache.InTimeRange(start, end, count)
	for _, s := range sts {
		nodeSts = append(nodeSts, s.(*NodeResourceState))
	}
	return nodeSts, nil
}

// GetNodeResourceRecentState get newest node resource state data
func (ns *nodeStoreManager) GetNodeResourceRecentState() (*NodeResourceState, error) {
	var cache *cadvisor.TimedStore

	err := func() error {
		var ok bool
		ns.lock.RLock()
		defer ns.lock.RUnlock()

		cache, ok = ns.caches[nodeResourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", nodeResourceName)
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	return cache.Get(0).(*NodeResourceState), nil
}

// Run main loop
func (ns *nodeStoreManager) Run(stop <-chan struct{}) {
	for _, collector := range ns.collectors {
		go wait.Until(collector.handle, ns.CollectInterval.TimeDuration(), stop)
		klog.V(2).Infof("%s collect %s started", nodeResourceName, collector.name)
	}
}

// GetNodeStoreSupportedTags list node state supported resource tag
func (ns *nodeStoreManager) GetNodeStoreSupportedTags() []string {
	return nodeStatExample.GetTags()
}

// addResourceStats add metric value to cache
func addResourceStats(name types.MetricKind, now time.Time, data interface{}) {
	var cache *cadvisor.TimedStore
	var ok bool

	func() {
		nodeManager.lock.Lock()
		defer nodeManager.lock.Unlock()

		cache, ok = nodeManager.caches[name]
		if !ok {
			cache = cadvisor.NewTimedStore(nodeManager.cacheAge, -1)
			nodeManager.caches[name] = cache
		}
	}()

	cache.Add(now, data)
}

// collectNodeResource collect resource metric one by one, we may collect in parallel if consume too much time
func collectNodeResource() {
	now := time.Now()
	nodeSt := &NodeResourceState{
		Timestamp: now,
		CPU:       collectCPU(),
		Load:      collectLoad(),
		Memory:    collectMemory(),
		Process:   collectProcess(),
	}
	if len(nodeManager.DiskNames) != 0 {
		nodeSt.DiskIO = collectDiskIO()
	}
	if len(nodeManager.Ifaces) != 0 {
		nodeSt.NetIO = collectNetIO()
	}
	if nodeSt.CPU != nil {
		addResourceStats(nodeResourceName, now, nodeSt)
	}
}

// collectCPU collect cpu usage
func collectCPU() *NodeCpu {
	usages, err := cpu.Times(true)
	if err != nil {
		klog.Errorf("cpu resource collect err: %v", err)
		return nil
	}
	if len(usages) == 0 {
		klog.Errorf("cpu resource collect is nil")
		return nil
	}

	if lastNodeResourceState.CPU == nil {
		lastNodeResourceState.CPU = &NodeCpu{
			state:     usages,
			timestamp: time.Now(),
		}
		return nil
	}

	nowCpuState := &NodeCpu{
		state:     usages,
		timestamp: time.Now(),
	}

	// calculate cpu consumption between the two timestamp
	total, perCore, totalSteal, stealPerCore := calculateCpuUsage(lastNodeResourceState.CPU, nowCpuState)
	lastNodeResourceState.CPU = nowCpuState
	if cpuNum == 0 {
		cpuNum = float64(len(usages))
	}

	return &NodeCpu{
		CpuTotal:        total,
		CpuPerCore:      perCore,
		CpuAvg:          total / cpuNum,
		CpuStealTotal:   totalSteal,
		CpuStealPerCore: stealPerCore,
	}
}

// collectLoad collect cpu load
func collectLoad() *NodeLoad {
	avg, err := load.Avg()
	if err != nil {
		klog.Errorf("cpu load resource collect err: %v", err)
		return nil
	}

	return &NodeLoad{
		Load1Min:  avg.Load1,
		Load5Min:  avg.Load5,
		Load15Min: avg.Load15,
	}
}

// collectMemory collect memory
func collectMemory() *NodeMemory {
	mem := procmeminfo.MemInfo{}
	err := mem.Update()
	if err != nil {
		klog.Errorf("mem resource collect err: %v", err)
		return nil
	}

	return &NodeMemory{
		Total:      float64(mem["MemTotal"]),
		UsageTotal: float64(mem["MemTotal"] - mem["MemFree"]),
		UsageRss:   float64(mem["MemTotal"] - mem["MemFree"] - mem["Buffers"] - mem["Cached"] + mem["Shmem"]),
		UsageCache: float64(mem["Buffers"] + mem["Cached"] - mem["Shmem"]),
		Available:  float64(mem["MemFree"] + mem["Buffers"] + mem["Cached"] - mem["Shmem"]),
	}
}

// collectDiskIO collect disk io
func collectDiskIO() *NodeDiskIO {
	if len(nodeManager.DiskNames) == 0 {
		return nil
	}
	ios, err := disk.IOCounters(nodeManager.DiskNames...)
	if err != nil {
		klog.Errorf("disk io resource collect err: %v", err)
		return nil
	}

	if lastNodeResourceState.DiskIO == nil {
		lastNodeResourceState.DiskIO = &NodeDiskIO{
			state:     ios,
			timestamp: time.Now(),
		}
		return nil
	}

	nowDiskIOState := &NodeDiskIO{
		state:     ios,
		timestamp: time.Now(),
	}
	// calculate disk io consumption between the two timestamp
	ioUsage := calculateDiskIO(lastNodeResourceState.DiskIO, nowDiskIOState)
	lastNodeResourceState.DiskIO = nowDiskIOState

	return &NodeDiskIO{
		IOState: ioUsage,
	}
}

// collectNetIO collect network io
func collectNetIO() *NodeNetwork {
	if len(nodeManager.Ifaces) == 0 {
		return nil
	}
	nets, err := net.IOCounters(true)
	if err != nil {
		klog.Errorf("net io resource collect err: %v", err)
		return nil
	}

	netIOStat := map[string]*net.IOCountersStat{}
	for i := range nets {
		if ifaces.Has(nets[i].Name) {
			netIOStat[nets[i].Name] = &nets[i]
		}
	}
	if len(netIOStat) == 0 {
		klog.Errorf("no network data collected for %v", nodeManager.Ifaces)
		return nil
	}

	if lastNodeResourceState.NetIO == nil {
		lastNodeResourceState.NetIO = &NodeNetwork{
			rawState:  netIOStat,
			timestamp: time.Now(),
		}
		return nil
	}

	nowNetState := &NodeNetwork{
		rawState:  netIOStat,
		timestamp: time.Now(),
	}
	calculateNetIO(lastNodeResourceState.NetIO, nowNetState)
	lastNodeResourceState.NetIO = nowNetState

	return nowNetState
}

// collectProcess collect process state
func collectProcess() *NodeProcess {
	path, err := getOfflineDevicesCgroupPath()
	if err != nil {
		klog.V(5).Infof("Failed to get memory cgroup path for offline tasks: %v", err)
		return nil
	}

	if netlinkReader == nil {
		var err error
		netlinkReader, err = netlink.New()
		if err != nil {
			klog.Errorf("Failed to create net link reader: %s", err)
			return nil
		}
	}

	stats, err := netlinkReader.GetCpuLoad("offlineLoad", path)
	if err != nil {
		klog.Errorf("Error getting process load for %q: %s", path, err)
		if strings.Contains(err.Error(), "no such file or directory") {
			// offline path not existed, need to check again
			offlinePodUid = ""
			offlineConPath = ""
		}
		return nil
	}
	klog.V(5).Infof("Process load for %s: %+v", path, stats)

	return &NodeProcess{
		NrUninterruptible: stats.NrUninterruptible,
	}
}

// getAllAndBusy get cpu usage
func getAllAndBusy(t cpu.TimesStat) (float64, float64) {
	busy := t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq
	return busy + t.Idle, busy
}

// calculateCpuUsage calculate cpu usage
func calculateCpuUsage(t1, t2 *NodeCpu) (total float64, perCore []float64, totalSteal float64, perCoreSteal []float64) {
	total = 0
	perCore = []float64{}
	totalSteal = 0
	perCoreSteal = []float64{}

	if len(t1.state) != len(t2.state) {
		return
	}

	nanoSeconds := t2.timestamp.UnixNano() - t1.timestamp.UnixNano()
	for i := range t1.state {
		_, t1Busy := getAllAndBusy(t1.state[i])
		_, t2Busy := getAllAndBusy(t2.state[i])

		if t2Busy <= t1Busy {
			continue
		}
		usageNanoCores := (t2Busy - t1Busy) * float64(time.Second/time.Nanosecond) / float64(nanoSeconds)
		usageNanoCores = float64(int64(100*usageNanoCores+0.5)) / 100
		perCore = append(perCore, usageNanoCores)
		total += usageNanoCores

		t1Steal := t1.state[i].Steal
		t2Steal := t2.state[i].Steal
		stealNanoCores := (t2Steal - t1Steal) * float64(time.Second/time.Nanosecond) / float64(nanoSeconds)
		stealNanoCores = float64(int64(100*stealNanoCores+0.5)) / 100
		perCoreSteal = append(perCoreSteal, stealNanoCores)
		totalSteal += stealNanoCores
	}

	return total, perCore, totalSteal, perCoreSteal
}

// calculateDiskIO calculate disk io usage
func calculateDiskIO(t1, t2 *NodeDiskIO) (ios map[string]DiskIOState) {
	ios = make(map[string]DiskIOState)

	duration := float64(t2.timestamp.Unix() - t1.timestamp.Unix())
	for disk := range t1.state {
		ios[disk] = DiskIOState{
			DiskReadKiBps:  float64(t2.state[disk].ReadBytes-t1.state[disk].ReadBytes) / 1024.0 / duration,
			DiskWriteKiBps: float64(t2.state[disk].WriteBytes-t1.state[disk].WriteBytes) / 1024.0 / duration,
			DiskReadIOps:   float64(t2.state[disk].ReadCount-t1.state[disk].ReadCount) / duration,
			DiskWriteIOps:  float64(t2.state[disk].WriteCount-t1.state[disk].WriteCount) / duration,
			// Util% is in percent unit
			Util: (float64(t2.state[disk].IoTime-t1.state[disk].IoTime) / 1000.0 / duration) * 100,
		}
	}

	return ios
}

// calculateNetIO calculate network io usage
func calculateNetIO(t1, t2 *NodeNetwork) {
	duration := float64(t2.timestamp.Unix() - t1.timestamp.Unix())
	t2.IfaceStats = map[string]*IfaceStat{}
	for iface, s2 := range t2.rawState {
		s1, ok := t1.rawState[iface]
		if !ok {
			continue
		}

		ifaceStat := &IfaceStat{
			Iface:        iface,
			Speed:        machine.GetIfaceSpeed(iface),
			NetRecvkbps:  float64(s2.BytesRecv-s1.BytesRecv) * 8 / 1000 / duration,
			NetSentkbps:  float64(s2.BytesSent-s1.BytesSent) * 8 / 1000 / duration,
			NetRecvPckps: float64(s2.PacketsRecv-s1.PacketsRecv) / duration,
			NetSentPckps: float64(s2.PacketsSent-s1.PacketsSent) / duration,
			NetDropIn:    float64(s2.Dropin-s1.Dropin) / duration,
			NetDropOut:   float64(s2.Dropout-s1.Dropout) / duration,
			NetFifoIn:    float64(s2.Fifoin-s1.Fifoin) / duration,
			NetFifoOut:   float64(s2.Fifoout-s1.Fifoout) / duration,
			NetErrin:     float64(s2.Errin-s1.Errin) / duration,
			NetErrout:    float64(s2.Errout-s1.Errout) / duration,
		}
		t2.IfaceStats[iface] = ifaceStat
	}
}

// getOfflineDevicesCgroupPath get the devices cgroup path for offline jobs
func getOfflineDevicesCgroupPath() (string, error) {
	root := cgroup.GetRoot()

	switch nodeManager.OfflineType {
	case types.OfflineTypeOnk8s:
		// the Qos module has already moving all offline pids the cgroup path
		if err := cgroup.EnsureCgroupPath(types.CgroupOffline); err != nil {
			klog.Errorf("failed init offline cgroups")
			return "", err
		}
		return filepath.Join(root, cgroup.DevicesSubsystem, types.CgroupOffline), nil
	case types.OfflineTypeYarnOnk8s:
		err := getOfflinePodCgroupPath()
		if err != nil {
			return "", err
		}
		return filepath.Join(root, cgroup.DevicesSubsystem, offlineConPath), nil
	}

	return "", fmt.Errorf("no offline cgroup found")
}

// getOfflinePodCgroupPath get nodemanager container path
func getOfflinePodCgroupPath() error {
	pods := nodeManager.podInformer.GetStore().List()
	for _, p := range pods {
		pod := p.(*v1.Pod)
		if appclass.IsOffline(pod) {
			if string(pod.UID) == offlinePodUid && len(offlineConPath) != 0 {
				return nil
			}
			containerPaths := appclass.PodCgroupDirs(pod)
			if len(containerPaths) != 1 {
				return fmt.Errorf("container path should just 1: %v", containerPaths)
			}
			offlinePodUid = string(pod.UID)
			offlineConPath = containerPaths[0]
			return nil
		}
	}
	return fmt.Errorf("no offline pod found")
}
