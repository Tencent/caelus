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
	"path"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
	global "github.com/tencent/caelus/pkg/types"

	"github.com/parnurzeal/gorequest"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

var (
	minCapacity  *global.NMCapacity
	initingStats = sets.NewString("NEW", "LOCALIZING", "LOCALIZED")
	runningStats = sets.NewString("RUNNING")
)

const (
	nmRequestStart               = "start"
	nmRequestStop                = "stop"
	nmRequestContainers          = "containers"
	nmRequestContainerstats      = "containerstats"
	nmRequestProperty            = "property"
	nmRequestCapacity            = "capacity"
	nmRequestScheduleDisable     = "scheduleDisable"
	nmRequestScheduleEnable      = "scheduleEnable"
	nmRequestCapacityUpdate      = "capacityUpdate"
	nmRequestCapacityForceUpdate = "capacityForceUpdate"
	nmRequestContainersKill      = "kill"
	nmRequestDiskPartitions      = "diskPartitions"
)

// GInitInterface is the interface used to contact with nm operator
type GInitInterface interface {
	// GetStatus check if node manager container is ready
	GetStatus() (bool, error)
	// StartNodemanager start node manager
	StartNodemanager() error
	// StopNodemanager stop node manager
	StopNodemanager() error
	// GetProperty get property map based on keys
	GetProperty(fileName string, keys []string, allKeys bool) (map[string]string, error)
	// SetProperty set property value
	SetProperty(fileName string, properties map[string]string, addNewKeys, delNonExistedKeys bool) error
	// GetCapacity return nodemanager resource capacity
	GetCapacity() (*global.NMCapacity, error)
	// SetCapacity set nodemanager resource capacity
	SetCapacity(capacity *global.NMCapacity) error
	// EnsureCapacity set capacity resource, and kill containers if necessary
	EnsureCapacity(expect *global.NMCapacity, conflictingResources []string, decreaseCap, scheduleDisabled bool)
	// WatchForMetricsPort watch changes of nodemanager metrics port, it is not thread safe
	WatchForMetricsPort() chan int
	// GetNMWebappPort get nodemanager webapp port
	GetNMWebappPort() (*int, error)
	// GetAllocatedJobs return container list, including resources and job state
	GetAllocatedJobs() ([]types.OfflineJobs, error)
	// GetMinCapacity get minimum capacity for nodemanager
	GetMinCapacity() *global.NMCapacity
	// KillContainer kill at least one container
	KillContainer(conflictingResource v1.ResourceName)
	// DisableSchedule disable nodemanager accepting new jobs
	DisableSchedule() error
	// EnableSchedule recover nodemanager accepting new jobs
	EnableSchedule() error
	// UpdateNodeCapacity update nodemanager capacity
	// Force updating when the node is in schedule disabled state if the force parameter is true
	UpdateNodeCapacity(capacity *global.NMCapacity, force bool) error
	// GetDiskPartitions get all disk partitions name
	GetDiskPartitions() ([]string, error)
}

// GInit group the options for contracting with nodemanager
type GInit struct {
	types.YarnNodeResourceConfig
	disableKillIfNormal         bool
	onlyKillIfIncompressibleRes bool
	metricsPortChan             chan int
}

// NewGInit create client based on given nodemanager address
func NewGInit(disableKillIfNormal, onlyKillIfIncompressibleRes bool, config types.YarnNodeResourceConfig) GInitInterface {
	return &GInit{
		disableKillIfNormal:         disableKillIfNormal,
		onlyKillIfIncompressibleRes: onlyKillIfIncompressibleRes,
		YarnNodeResourceConfig:      config,
	}
}

// GetStatus check if node manager container is ready
func (g *GInit) GetStatus() (bool, error) {
	var nmStatus global.NMStatus

	resp, data, errs := newRequest().Get(g.NMServer + "/v1/nm/status").EndStruct(&nmStatus)
	if len(errs) > 0 {
		return false, fmt.Errorf("get staus failed: %v, response body %s", errs, string(data))
	}
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("get status failed %d: %s, response body %s", resp.StatusCode, resp.Status, string(data))
	}

	if nmStatus.State == "DOWN" {
		klog.Errorf("nodemanager is DOWN: %s", nmStatus.Description)
		return false, nil
	}

	return true, nil
}

// StartNodemanager start node manager
func (g *GInit) StartNodemanager() error {
	if g.PortAutoDetect {
		if err := g.ensurePort(); err != nil {
			return fmt.Errorf("ensure nm port: %v", err)
		}
	} else {
		// if port auto detect is closed, need to notify the right metric port to metrics module
		g.sendMetricsPort()
	}

	// DiskSpaceLimited is the temporary code for old NM image, and will drop in feature
	// if all partitions' size are too small, just set nodemanager as unhealthy by the disk health checker
	if DiskSpaceLimited {
		g.Properties["yarn.nodemanager.disk-health-checker.min-free-space-per-disk-mb"] = "10240000"
	}

	if !g.EnableYarnQOS {
		// no need to set resource limit by nodemanager, just limited by the QOS module
		klog.V(2).Infof("disable nodemanager QOS function")
		properties := map[string]string{
			"yarn.nodemanager.resource.percentage-physical-cpu-limit": "100",
			"yarn.nodemanager.resource.memory.enabled":                "false"}
		// set extra properties from config file
		if len(g.Properties) != 0 {
			for k, v := range properties {
				g.Properties[k] = v
			}
		} else {
			g.Properties = properties
		}
	}
	if len(g.Properties) > 0 {
		if err := g.SetProperty(YarnSite, g.Properties, true, false); err != nil {
			return err
		}
	}

	klog.V(2).Infof("starting nodemanager process")
	return g.sendRequest(nmRequestStart, "POST", nil, nil)
}

// StopNodemanager stop node manager
func (g *GInit) StopNodemanager() error {
	klog.V(2).Infof("stopping nodemanager process")
	return g.sendRequest(nmRequestStop, "POST", nil, nil)
}

// GetProperty get property map based on keys
// - fileName: which xml file to check
// - keys: return values for which keys
// - allKeys: if need to return values for all keys
func (g *GInit) GetProperty(fileName string, keys []string,
	allKeys bool) (map[string]string, error) {
	var configProperties global.ConfigProperty
	var configKeys = global.ConfigKey{
		ConfigFile: fileName,
		ConfigKeys: keys,
		ConfigAll:  allKeys,
	}

	err := g.sendRequest(nmRequestProperty, "GET", &configKeys, &configProperties)
	return configProperties.ConfigProperty, err
}

// SetProperty set property value
// - fileName: which xml file to check
// - properties: properties need to set
// - addNewKeys: if need to add keys when keys not existed
// - delNonExistedKeys: if need to delete keys not existed in properties
func (g *GInit) SetProperty(fileName string, properties map[string]string,
	addNewKeys, delNonExistedKeys bool) error {
	var configProperties = global.ConfigProperty{
		ConfigFile:     fileName,
		ConfigProperty: properties,
		ConfigAdd:      addNewKeys,
		ConfigDel:      delNonExistedKeys,
	}

	return g.sendRequest(nmRequestProperty, "POST", &configProperties, nil)
}

// GetCapacity return nodemanager resource capacity
func (g *GInit) GetCapacity() (*global.NMCapacity, error) {
	capacity := &global.NMCapacity{}

	err := g.sendRequest(nmRequestCapacity, "GET", nil, capacity)
	if err == nil {
		capacity.Millcores = capacity.Vcores * types.CPUUnit
	}
	return capacity, err
}

// SetCapacity set nodemanager resource capacity
// we could not set zero with capacity API, so changing to property API
func (g *GInit) SetCapacity(capacity *global.NMCapacity) error {
	properties := map[string]string{
		"yarn.nodemanager.resource.memory-mb":  strconv.Itoa(int(capacity.MemoryMB)),
		"yarn.nodemanager.resource.cpu-vcores": strconv.Itoa(int(capacity.Vcores)),
	}
	return g.SetProperty(YarnSite, properties, false, false)
}

// GetMinCapacity return minimum allowed capacity
func (g *GInit) GetMinCapacity() *global.NMCapacity {
	if minCapacity != nil {
		return minCapacity
	}

	// initialize original value if min property not set
	// the nodemanger could assign minimum cpu value as zero
	cap := &global.NMCapacity{
		Vcores:    0,
		Millcores: 0,
		MemoryMB:  1024,
	}

	var keys = []string{
		"yarn.scheduler.minimum-allocation-mb",
	}
	values, err := g.GetProperty(YarnSite, keys, false)
	if err != nil {
		klog.Errorf("request min capacity err: %v", err)
		// just return the default value
		return cap
	}

	if values["yarn.scheduler.minimum-allocation-mb"] != "" {
		memMB, err := strconv.Atoi(values["yarn.scheduler.minimum-allocation-mb"])
		if err != nil {
			klog.Fatalf("invalid yarn.scheduler.minimum-allocation-mb: %s",
				values["yarn.scheduler.minimum-allocation-mb"])
		}
		// the default minimum allocation memory is 128Mb, it is too small to cause oom
		// so we set memory at least 1024Mb
		if int64(memMB) > cap.MemoryMB {
			cap.MemoryMB = int64(memMB)
		}
	}

	minCapacity = cap
	return minCapacity
}

// GetAllocatedJobs return container list, including resources and job state
func (g *GInit) GetAllocatedJobs() ([]types.OfflineJobs, error) {
	offlineJobs := []types.OfflineJobs{}
	conList, err := g.getNMContainers()
	if err != nil {
		return offlineJobs, err
	}

	for _, con := range conList {
		if runningStats.Has(con.State) || initingStats.Has(con.State) {
			metadata := YarnMetadata{
				Name: con.ID,
			}
			offlineJobs = append(offlineJobs, types.OfflineJobs{
				Metadata: &metadata,
				// has not assign resource quantity
			})
		}
	}

	return offlineJobs, nil
}

// EnsureCapacity set capacity resource, and kill containers if necessary.
func (g *GInit) EnsureCapacity(expect *global.NMCapacity, conflictingResources []string, decreaseCap, scheduleDisabled bool) {
	nodeCap := &global.NMCapacity{
		Vcores:    expect.Vcores,
		Millcores: expect.Millcores,
		MemoryMB:  expect.MemoryMB,
	}
	if scheduleDisabled {
		nodeCap.Vcores = 0
	}
	err := g.UpdateNodeCapacity(nodeCap, false)
	if err != nil {
		klog.Errorf("update yarn capacity(%v) without force err: %v", expect, err)
	}

	// if disableKillIfNormal is enabled, no need to kill pod when no conflicting resource
	if len(conflictingResources) == 0 && g.disableKillIfNormal {
		klog.Warning("no conflicting resource found, no need to kill containers")
		return
	}
	if len(conflictingResources) > 0 {
		if g.onlyKillIfIncompressibleRes && types.AllResCompressible(conflictingResources) {
			klog.V(2).Infof("all conflict resources(%v) are compressible, skip kill containers", conflictingResources)
			return
		}
	}
	// need to kill offline jobs when in conflicting state or decreasing capacity
	if len(conflictingResources) > 0 || decreaseCap {
		conflictingResource := ""
		if len(conflictingResources) != 0 {
			conflictingResource = conflictingResources[0]
			klog.V(2).Infof("kill containers for interference: %v", conflictingResources)
		} else {
			klog.V(2).Infof("kill containers because capacity decreasing")
		}
		err = g.killContainers(expect, v1.ResourceName(conflictingResource))
		if err != nil {
			klog.Errorf("kill containers after setting capacity failed: %v", err)
		}
	}
}

// KillContainer kill at least one container
func (g *GInit) KillContainer(conflictingResource v1.ResourceName) {
	g.killContainers(nil, conflictingResource)
}

// DisableSchedule disable nodemanager accepting new jobs
// this may cost 2s!
func (g *GInit) DisableSchedule() error {
	return g.sendRequest(nmRequestScheduleDisable, "POST", nil, nil)
}

// EnableSchedule recover nodemanager accepting new jobs
// this may cost 2s!
func (g *GInit) EnableSchedule() error {
	return g.sendRequest(nmRequestScheduleEnable, "POST", nil, nil)
}

// UpdateNodeCapacity update nodemanager capacity,
// Force updating when the node is in schedule disabled state if the force parameter is true
func (g *GInit) UpdateNodeCapacity(capacity *global.NMCapacity, force bool) error {
	action := nmRequestCapacityUpdate
	if force {
		action = nmRequestCapacityForceUpdate
	}
	// the update request could cost 2s, so make it asynchronous
	go func() {
		var i = 0
		for {
			err := g.sendRequest(action, "POST", capacity, nil)
			if err == nil {
				break
			}

			i++
			if i == 2 {
				klog.Errorf("Ginit update capacity(%+v) err after %d times: %v", capacity, i, err)
				break
			}
			time.Sleep(1 * time.Second)
		}
	}()

	return nil
}

// killContainers kill containers based on container stats, such as resource usages
func (g *GInit) killContainers(expect *global.NMCapacity, conflictingResource v1.ResourceName) error {
	var err error
	containers := []global.NMContainer{}

	containers, err = g.getNMContainers()
	if err != nil {
		// just warning, for we need to set nm capacity
		klog.Errorf("get all containers failed: %v", err)
		return err
	}

	cids := []string{}
	for _, con := range containers {
		cids = append(cids, con.ID)
	}
	contsState, err := g.getNMContainersState(cids)
	if err != nil {
		klog.Errorf("get container stats err: %v", err)
		contsState = make(map[string]*global.ContainerState)
	} else {
		if klog.V(4) {
			klog.Info("newest container stats: ")
			for cid, state := range contsState {
				klog.Infof("   cid: %s, state: %+v", cid, state)
			}
		}
	}

	return g.killContainersWithStats(containers, contsState, expect, conflictingResource)
}

// killContainersWithStats will pick some containers to kill
func (g *GInit) killContainersWithStats(containers []global.NMContainer, contsState map[string]*global.ContainerState,
	expect *global.NMCapacity, conflictingResource v1.ResourceName) error {
	allocatedRes, usedRes, runningContainers := g.getContainerResource(containers, contsState, conflictingResource)

	if conflictingResource == v1.ResourceMemory {
		sort.Sort(byAmAndMemory(runningContainers))
	} else if conflictingResource == v1.ResourceCPU {
		sort.Sort(byAmAndCPU(runningContainers))
	} else {
		sort.Sort(byAmAndTime(runningContainers))
	}
	klog.V(2).Info("running sorted containers:")
	for i := range runningContainers {
		klog.V(2).Info(runningContainers[i])
	}

	// get current nodemanager container cgroup path
	yarnPath := getYarnCgroupPath()

	// pick which containers to kill
	var picked []global.NMContainer
	for i := range runningContainers {
		if expect == nil || (allocatedRes.memoryMB <= float32(expect.MemoryMB) &&
			allocatedRes.vcores <= float32(expect.Vcores) &&
			usedRes.memoryMB <= float32(expect.MemoryMB) && usedRes.vcores <= float32(expect.Vcores)) {
			// if resource conflicting, then should kill at least one job
			if conflictingResource == "" {
				klog.V(2).Infof("no resource conflicting," +
					"and both allocated and used resources are ok, won't kill containers")
				break
			}
			if len(picked) != 0 {
				break
			}
		}

		gpid := getContainerProcessGroupId(yarnPath, runningContainers[i].ID)
		if gpid <= 1 {
			continue
		}
		runningContainers[i].Pid = gpid

		picked = append(picked, runningContainers[i])
		allocatedRes.memoryMB -= float32(runningContainers[i].TotalMemoryNeededMB)
		allocatedRes.vcores -= runningContainers[i].TotalVCoresNeeded
		usedRes.memoryMB -= runningContainers[i].UsedMemoryMB
		usedRes.vcores -= runningContainers[i].UsedVCores
	}

	klog.V(2).Infof("need to kill %d sorted containers from all %d", len(picked), len(runningContainers))
	cids := []string{}
	for _, con := range picked {
		metrics.KillCounterInc("yarn")
		klog.V(2).Infof("killing nm container %+v", con)
		cids = append(cids, con.ID)
		err := g.requestKillNMContainers(cids)
		if err != nil {
			klog.Errorf("request kill container list err: %v", err)
		}
	}
	return nil
}

// getContainerResource return total allocated and used resources, also the running container list.
func (g *GInit) getContainerResource(containers []global.NMContainer, contsState map[string]*global.ContainerState,
	conflictingResource v1.ResourceName) (allocatedRes, usedRes *commonResource,
	runningContainers []global.NMContainer) {
	allocatedRes = &commonResource{
		vcores:   0,
		memoryMB: 0,
	}
	usedRes = &commonResource{
		vcores:   0,
		memoryMB: 0,
	}
	for i := range containers {
		if runningStats.Has(containers[i].State) {
			// get container real usage resources
			if st, ok := contsState[containers[i].ID]; ok {
				containers[i].UsedMemoryMB = st.UsedMemMB
				containers[i].UsedVCores = st.UsedCPU
				containers[i].StartTime = st.StartTime
			} else {
				klog.Errorf("container(%s) state not found", containers[i].ID)
			}
			runningContainers = append(runningContainers, containers[i])
			allocatedRes.memoryMB += float32(containers[i].TotalMemoryNeededMB)
			allocatedRes.vcores += containers[i].TotalVCoresNeeded
			klog.V(5).Infof("running container: %+v", containers[i])
		} else if initingStats.Has(containers[i].State) {
			allocatedRes.memoryMB += float32(containers[i].TotalMemoryNeededMB)
			allocatedRes.vcores += containers[i].TotalVCoresNeeded
			klog.V(2).Infof("non-running container: %+v", containers[i])
		} else {
			klog.V(4).Infof("container is not in starting or running: %+v", containers[i])
		}
	}
	klog.V(4).Infof("all containers allocated resource: %+v", allocatedRes)

	for i := range runningContainers {
		usedRes.memoryMB += runningContainers[i].UsedMemoryMB
		usedRes.vcores += runningContainers[i].UsedVCores
	}
	klog.V(4).Infof("all running containers used resource: %+v", usedRes)

	return allocatedRes, usedRes, runningContainers
}

// GetNMContainers return all containers on the node
func (g *GInit) getNMContainers() ([]global.NMContainer, error) {
	containers := &global.NMContainersWrapper{}
	err := g.sendRequest(nmRequestContainers, "GET", nil, containers)
	return containers.Container, err
}

// GetNMContainersState get all containers resource usage and pid list based on container ids
func (g *GInit) getNMContainersState(cids []string) (map[string]*global.ContainerState, error) {
	containers := &global.NMContainerIds{
		Cids: cids,
	}

	cstats := struct {
		Cstats map[string]*global.ContainerState `json:"cstats"`
	}{Cstats: make(map[string]*global.ContainerState)}

	err := g.sendRequest(nmRequestContainerstats, "GET", containers, &cstats)
	return cstats.Cstats, err
}

// requestKillNMContainers post container list to evict
func (g *GInit) requestKillNMContainers(cids []string) error {
	containers := &global.NMContainerIds{
		Cids: cids,
	}

	return g.sendRequest(nmRequestContainersKill, "POST", containers, nil)
}

func (g *GInit) sendRequest(action string, method string, sendBody, respBody interface{}) error {
	var resp gorequest.Response
	var errs []error
	var data string
	var dataBytes []byte

	url, err := actionURL(action)
	if err != nil {
		return err
	}

	client := newRequest()
	switch method {
	case "GET":
		if sendBody != nil && respBody != nil {
			resp, dataBytes, errs = client.Get(g.NMServer + url).Send(sendBody).EndStruct(respBody)
			data = string(dataBytes)
		} else if respBody != nil {
			resp, dataBytes, errs = client.Get(g.NMServer + url).EndStruct(respBody)
			data = string(dataBytes)
		} else {
			resp, dataBytes, errs = client.Get(g.NMServer + url).EndBytes()
			data = string(dataBytes)
		}
	case "POST":
		if sendBody != nil {
			resp, data, errs = client.Post(g.NMServer + url).Send(sendBody).End()
		} else if respBody != nil {
			resp, dataBytes, errs = client.Post(g.NMServer + url).EndStruct(respBody)
			data = string(dataBytes)
		} else {
			resp, data, errs = client.Post(g.NMServer + url).End()
		}
	default:
		return fmt.Errorf("unsupported method")
	}
	if len(errs) > 0 {
		return fmt.Errorf("request action %s failed: %v, response body %s", action, errs, data)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("request action %s failed %d: %s, response body %s", action, resp.StatusCode, resp.Status, data)
	}
	return nil
}

// GetDiskPartitions get all disk partitions
func (g *GInit) GetDiskPartitions() ([]string, error) {
	nmDiskPartition := &global.NMDiskPartition{}
	err := g.sendRequest(nmRequestDiskPartitions, "GET", nil, nmDiskPartition)
	return nmDiskPartition.PartitionsName, err
}

func newRequest() *gorequest.SuperAgent {
	client := gorequest.New().Timeout(time.Minute)
	return client
}

func actionURL(action string) (url string, err error) {
	switch action {
	case nmRequestStart:
		url = "/v1/nm/start"
	case nmRequestStop:
		url = "/v1/nm/stop"
	case nmRequestContainers:
		url = "/v1/nm/containers"
	case nmRequestContainerstats:
		url = "/v1/nm/containerstats"
	case nmRequestProperty:
		url = "/v1/nm/property"
	case nmRequestCapacity:
		url = "/v1/nm/capacity"
	case nmRequestScheduleDisable:
		url = "/v1/nm/schedule/disable"
	case nmRequestScheduleEnable:
		url = "/v1/nm/schedule/enable"
	case nmRequestCapacityUpdate:
		url = "/v1/nm/capacity/update"
	case nmRequestCapacityForceUpdate:
		url = "/v1/nm/capacity/update/force"
	case nmRequestContainersKill:
		url = "/v1/nm/kill"
	case nmRequestDiskPartitions:
		url = "/v1/nm/diskpartitions"
	default:
		err = fmt.Errorf("invalid action: %s", action)
	}
	return
}

func getYarnCgroupPath() string {
	pod, err := getOfflinePod()
	if err != nil {
		klog.Errorf("get offline pod failed when get yarn cgroup path: %v", err)
		return ""
	}
	if pod == nil {
		klog.Error("get nil offline pod when get yarn cgroup path")
		return ""
	}

	paths := appclass.PodCgroupDirs(pod)
	if len(paths) != 1 {
		klog.Errorf("invalid Yarn cgroup paths: %v", paths)
		return ""
	}

	return paths[0]
}

func getContainerProcessGroupId(parentCgPath, conID string) int {
	if len(parentCgPath) == 0 || len(conID) == 0 {
		klog.Errorf("get container process group id failed, not found parent cgroup")
		return 0
	}

	// get container pid list
	cgPath := path.Join(cgroup.GetRoot(), "cpu", parentCgPath, types.CgroupYarn, conID)
	pidList, err := cgroup.GetPids(cgPath)
	if err != nil {
		klog.Errorf("list cgroup(%s) pid err: %v", cgPath, err)
		return 0
	}
	if len(pidList) == 0 {
		klog.Errorf("list cgroup(%s) pid, got nil", cgPath)
		return 0
	}

	// get process group id
	gpid, err := syscall.Getpgid(pidList[0])
	if err != nil {
		klog.Errorf("get group id for process id(%d) err: %v", pidList[0], err)
		// shall we return the original pid: pidList[0]?
		return 0
	}
	return gpid
}
