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

package manager

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

var (
	minCpuBtPercent = 10
	cpusetRecovered = false
)

const (
	QosCPU      = "cpu"
	QosCPUBT    = "cpuBT"
	QosCPUSet   = "cpuSet"
	QosCPUQuota = "cpuQuota"

	QosAdaptive = "cpuAdaptive"

	tryDuration     = 15 * time.Minute
	collectDuration = 10 * time.Minute
	roundDuration   = 6 * time.Hour

	winThreshold = 5
)

type cpuManagerFactory func() ResourceQosManager

// qosCpuAdaptive choose cpu policy based on online job metrics
type qosCpuAdaptive struct {
	factories      []cpuManagerFactory
	managerMetrics []float64
	store          statestore.StateStore

	currTry           int
	currentManager    ResourceQosManager
	currentWinFactory int
	winCount          int

	lastCgResources *CgroupResourceConfig

	roundCount int
	sync.Mutex
}

// Name return name of qos manager
func (q *qosCpuAdaptive) Name() string {
	return QosAdaptive
}

// PreInit initialize all values
func (q *qosCpuAdaptive) PreInit() error {
	q.currentManager = q.factories[0]()
	q.currTry = 0
	return q.currentManager.PreInit()
}

// Run choose candidate cpu policy to confirm which policy is better for online jobs
func (q *qosCpuAdaptive) Run(stop <-chan struct{}) {
	go func() {
		klog.Infof("try using cpu policy %s", q.currentManager.Name())
		t := time.NewTimer(tryDuration)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				now := time.Now()
				start := now.Add(-collectDuration)
				metrics, err := q.store.ListCustomizeResourceRangeStats(start, now, -1)
				if err != nil || len(metrics) == 0 {
					klog.Warningf("no customize metric data")
					t.Reset(tryDuration + time.Duration(rand.Intn(5*60))*time.Second)
					continue
				}
				if len(metrics) > 1 {
					klog.Warningf("more than one custom metrics, use current cpu policy %s", q.currentManager.Name())
					return
				}
				sum := 0.0
				for _, vv := range metrics {
					cnt := 0
					for _, v := range vv {
						value, exist := v.Values["value"]
						if !exist {
							continue
						}
						cnt++
						sum += value
					}
					if cnt > 0 {
						sum /= float64(cnt)
					}
				}

				q.managerMetrics[q.currTry] = sum
				klog.Infof("policy %s metric %v", q.currentManager.Name(), sum)

				q.currTry = (q.currTry + 1) % len(q.factories)
				if q.currTry == q.currentWinFactory {
					q.roundCount++
					min := q.managerMetrics[0]
					minIdx := 0
					for i, v := range q.managerMetrics {
						if v < min {
							minIdx = i
							min = v
						}
					}
					if q.currentWinFactory == minIdx {
						q.winCount++
						if q.winCount >= winThreshold {
							q.renewManager(stop)
							klog.Infof("final round(%d) select %s as cpu policy winner",
								q.roundCount, q.currentManager.Name())
							return
						}
					} else {
						q.currentWinFactory = minIdx
						q.winCount = 1
						q.currTry = minIdx
					}
					q.renewManager(stop)
					klog.Infof("round(%d) select %s as cpu policy", q.roundCount, q.currentManager.Name())
					t.Reset(roundDuration + time.Duration(rand.Intn(30))*time.Minute)
					continue
				}
				q.renewManager(stop)
				klog.Infof("try using cpu policy %s", q.currentManager.Name())
				t.Reset(tryDuration + time.Duration(rand.Intn(5*60))*time.Second)
			case <-stop:
				return
			}
		}
	}()
}

func (q *qosCpuAdaptive) renewManager(stop <-chan struct{}) {
	q.Lock()
	defer q.Unlock()
	cpusetRecovered = false
	q.currentManager = q.factories[q.currTry]()
	q.currentManager.PreInit()
	q.currentManager.Run(stop)
	if q.lastCgResources != nil {
		q.currentManager.Manage(q.lastCgResources)
	}
}

// Manage manage cpu with current policy
func (q *qosCpuAdaptive) Manage(cgResources *CgroupResourceConfig) error {
	q.Lock()
	defer q.Unlock()
	q.lastCgResources = cgResources
	return q.currentManager.Manage(cgResources)
}

// NewQosCpuAdaptive new cpu adaptive instance
func NewQosCpuAdaptive(kubeletStatic bool, config types.CpuSetConfig, store statestore.StateStore) ResourceQosManager {
	adaptive := qosCpuAdaptive{
		factories: []cpuManagerFactory{
			func() ResourceQosManager {
				return NewQosCpuBT(kubeletStatic)
			},
			func() ResourceQosManager {
				return NewQosCpuSet(config)
			},
		},
		store:             store,
		currentWinFactory: 0,
	}
	adaptive.managerMetrics = make([]float64, len(adaptive.factories))

	return &adaptive
}

// qosCpuBT manage offline job by bt feature
type qosCpuBT struct {
	lastNCores    *int
	kubeletStatic bool
}

// NewQosCpuBT creates cpu manager with bt feature
func NewQosCpuBT(kubeletStatic bool) ResourceQosManager {
	return &qosCpuBT{
		kubeletStatic: kubeletStatic,
	}
}

// Name returns resource policy name
func (b *qosCpuBT) Name() string {
	return QosCPUBT
}

// PreInit initialize cgroup configuration and enable offline feature
func (b *qosCpuBT) PreInit() error {
	err := initCgroup()
	if err != nil {
		return err
	}

	if err := cgroup.CPUOfflineSet(types.CgroupOffline, true); err != nil {
		return fmt.Errorf("enable cgroup(%s) offline feature err: %v",
			types.CgroupOffline, err)
	}
	// should set offline feature for all child cgroups, which may not set
	if err := cgroup.CPUChildOfflineSet(types.CgroupOffline, true); err != nil {
		return fmt.Errorf("enable cgroup(%s) child offline feature err: %v",
			types.CgroupOffline, err)
	}

	// recover cpu quota
	err = cgroup.SetCpuQuota(types.CgroupOffline, -1)
	if err != nil {
		klog.Errorf("cpuset recover quota err: %v", err)
		return err
	}

	return nil
}

// Run starts nothing
func (b *qosCpuBT) Run(stop <-chan struct{}) {}

// Manage manages cpus with bt feature
// offline jobs could use all cores, which limited by bt feature
func (b *qosCpuBT) Manage(cgResources *CgroupResourceConfig) error {
	nCores := getOfflineCores(cgResources.Resources)
	if nCores == 0 {
		klog.Warningf("cpu isolating resource is zero")
		return nil
	}

	if b.lastNCores != nil && *b.lastNCores == nCores {
		klog.V(4).Info("qos for cpu no changed, nothing to to")
		return nil
	}

	// setting parent cgroup is ok, for new child cgroups will inherit the offline feature
	offlineParent := types.CgroupOffline
	err := cgroup.CPUOfflineSet(offlineParent, true)
	if err != nil {
		klog.Errorf("enable cgroup(%s) cpu offline failed: %v", offlineParent, err)
		b.lastNCores = nil
		return err
	}

	// limit offline tasks to limited usage for all cores
	err = cgroup.CPUOfflineLimit(nCores, minCpuBtPercent)
	if err != nil {
		b.lastNCores = nil
		return fmt.Errorf("limit cpu usage failed: %v", err)
	}

	// the lighthouse plugin will set 0 to offline cpuset cgroup when kubelet cpu manager is static,
	// we need to recover
	noError := true
	if b.kubeletStatic || !cpusetRecovered {
		errs := recoverCoresForCgroups(cgResources.OfflineCgroups)
		if len(errs) != 0 {
			noError = false
			cpusetRecovered = true
		}
	}

	// record the value
	if noError {
		b.lastNCores = &nCores
	} else {
		// clear record to handle again next time
		b.lastNCores = nil
	}

	return nil
}

// qosCpuSet manage offline job by cpuset feature
type qosCpuSet struct {
	lastNCores     *int
	lastOfflineCgs *cgroupPaths
	lastOnlineCgs  *cgroupPaths
	onlineIsolate  bool
	reserved       sets.Int
}

// NewQosCpuSet creates cpu manager with cpuset feature
func NewQosCpuSet(config types.CpuSetConfig) ResourceQosManager {
	reserved, err := parseCpList(config.ReservedCpus)
	if err != nil {
		klog.Fatalf("invalid reserved cpu format(%s): %v", config.ReservedCpus, err)
	}
	klog.V(2).Infof("reserved cpus list(%d): %v", reserved.Len(), reserved)

	return &qosCpuSet{
		onlineIsolate:  config.EnableOnlineIsolate,
		reserved:       reserved,
		lastOfflineCgs: newCgroupPaths(),
		lastOnlineCgs:  newCgroupPaths(),
	}
}

// Name returns resource policy name
func (s *qosCpuSet) Name() string {
	return QosCPUSet
}

// PreInit initialize cgroup configuration
func (s *qosCpuSet) PreInit() error {
	err := initCgroup()
	if err != nil {
		return err
	}

	// recover cpu quota
	err = cgroup.SetCpuQuota(types.CgroupOffline, -1)
	if err != nil {
		klog.Errorf("cpuset recover quota err: %v", err)
		return err
	}

	if cgroup.CPUOfflineSupported() {
		if err := cgroup.CPUOfflineSet(types.CgroupOffline, false); err != nil {
			return fmt.Errorf("recover cgroup(%s) offline feature err: %v",
				types.CgroupOffline, err)
		}
		// should set offline feature for all child cgroups, which may not set
		if err := cgroup.CPUChildOfflineSet(types.CgroupOffline, false); err != nil {
			return fmt.Errorf("recover cgroup(%s) child offline feature err: %v",
				types.CgroupOffline, err)
		}
	}
	return err
}

// Run starts nothing
func (s *qosCpuSet) Run(stop <-chan struct{}) {}

// Manage manages cpus with cpuset feature
// offline jobs can only use part of cores, and also could isolate with online jobs.
// there is a problem for setting cores for offline jobs at creating time,
// so we let light plugin to set just one core for offline job at creating time,
// then the function changed it to right cores.
func (s *qosCpuSet) Manage(cgResources *CgroupResourceConfig) error {
	nCores := getOfflineCores(cgResources.Resources)
	if nCores == 0 {
		klog.Warningf("cpu isolating resource is zero")
		return nil
	}

	if s.lastNCores != nil {
		if *s.lastNCores == nCores && s.lastOfflineCgs.equal(cgResources.OfflineCgroups) {
			if !s.onlineIsolate ||
				(s.onlineIsolate && s.lastOnlineCgs.equal(cgResources.OnlineCgroups)) {
				klog.V(4).Info("qos for cpu no changed, nothing to to")
				return nil
			}
		}
	}

	// get total cores from root cgroup
	totalCoresStr, err := cgroup.GetCpuSet("", true)
	if err != nil {
		klog.Errorf("parse cpuset from root cgroup err: %v", err)
		s.lastNCores = nil
		return err
	}
	var totalCores []int
	for _, cStr := range totalCoresStr {
		c, _ := strconv.Atoi(cStr)
		// remove reserved cores
		if !s.reserved.Has(c) {
			totalCores = append(totalCores, c)
		}
	}

	offlineCores, leftCores := cgroup.ChooseNumaCores(totalCores, nCores)
	klog.V(4).Infof("isolate %d cores, total: %v, offline: %v, online: %v",
		nCores, totalCores, offlineCores, leftCores)
	if len(offlineCores) == 0 {
		s.lastNCores = nil
		return fmt.Errorf("isolate %d cores from total %v, found zero for offline",
			nCores, totalCores)
	}

	//set offline cores to offline cgroups
	errs := setCoresForCgroups(cgResources.OfflineCgroups, offlineCores, "")
	if len(errs) == 0 {
		s.lastNCores = &nCores
		s.lastOfflineCgs.copy(cgResources.OfflineCgroups)
	}

	// recover cpu quota
	offlineParent := types.CgroupOffline
	err = cgroup.SetCpuQuota(offlineParent, -1)
	if err != nil {
		klog.Errorf("cpuset recover quota err: %v", err)
		s.lastNCores = nil
	}

	// shall online jobs use reserved cpus? if needed, we should add the reserved cpus
	if s.onlineIsolate {
		leftCores = append(leftCores, s.reserved.List()...)
		if len(leftCores) == 0 {
			s.lastNCores = nil
			return fmt.Errorf("isolate %d cores from total %v, found zero for online",
				nCores, totalCores)
		}

		klog.V(4).Infof("online jobs cpu isolate is enabled, isolating: %v", leftCores)
		errs = setCoresForCgroups(cgResources.OnlineCgroups, leftCores, "")
		if len(errs) == 0 {
			s.lastOnlineCgs.copy(cgResources.OnlineCgroups)
		} else {
			// clear record to handle again next time
			s.lastNCores = nil
		}
	}

	return nil
}

// qosCpuQuota manage offline job by quota/period feature
type qosCpuQuota struct {
	lastNCores     *int
	lastOfflineCgs *cgroupPaths
	shareWeight    *uint64
	kubeletStatic  bool
}

// NewQosCpuQuota creates cpu manager with quota/period feature
func NewQosCpuQuota(kubeletStatic bool, config types.CpuQuotaConfig) ResourceQosManager {
	return &qosCpuQuota{
		kubeletStatic:  kubeletStatic,
		shareWeight:    config.OfflineShare,
		lastOfflineCgs: newCgroupPaths(),
	}
}

// Name returns resource policy name
func (q *qosCpuQuota) Name() string {
	return QosCPUQuota
}

// PreInit do some initializations
func (q *qosCpuQuota) PreInit() error {
	err := initCgroup()
	if err != nil {
		return err
	}

	if cgroup.CPUOfflineSupported() {
		if err := cgroup.CPUOfflineSet(types.CgroupOffline, false); err != nil {
			return fmt.Errorf("recover cgroup(%s) offline feature err: %v",
				types.CgroupOffline, err)
		}
		// should recover offline feature for all child cgroups, which may miss offline feature
		if err := cgroup.CPUChildOfflineSet(types.CgroupOffline, false); err != nil {
			return fmt.Errorf("recover cgroup(%s) child offline feature err: %v",
				types.CgroupOffline, err)
		}
	}
	return err
}

// Run do nothing
func (q *qosCpuQuota) Run(stop <-chan struct{}) {}

// ManageCpu manages cpus with quota/period feature
// offline jobs could use all of cores, while is limited by quota/period, and also lower weight
func (q *qosCpuQuota) Manage(cgResources *CgroupResourceConfig) error {
	nCores := getOfflineCores(cgResources.Resources)
	if nCores == 0 {
		klog.Warningf("cpu isolating resource is zero")
		return nil
	}

	if q.lastNCores != nil && *q.lastNCores == nCores {
		if q.shareWeight == nil ||
			(q.shareWeight != nil && q.lastOfflineCgs.equal(cgResources.OfflineCgroups)) {
			klog.V(4).Info("qos for cpu no changed, nothing to to")
			return nil
		}
	}

	offlineParent := types.CgroupOffline
	klog.V(4).Infof("setting cpu quota for %s: %d", offlineParent, nCores)
	err := cgroup.SetCpuQuota(offlineParent, float64(nCores))
	if err != nil {
		// clear record to handle next time
		q.lastNCores = nil
		return err
	}
	q.lastNCores = &nCores

	noError := true
	// set cpu weight for offline tasks
	if q.shareWeight != nil {
		klog.V(4).Infof("setting cpu shares value(%d) for offline cgroups", *q.shareWeight)
		errs := setSharesForCgroups(cgResources.OfflineCgroups, *q.shareWeight)
		if len(errs) != 0 {
			noError = false
		}
	} else {
		klog.V(4).Infof("offline cpu shares value is nil, just ignore")
	}

	// the lighthouse plugin will set 0 to offline cpuset cgroup when kubelet cpu manager is static,
	// we need to recover
	if q.kubeletStatic || !cpusetRecovered {
		errs := recoverCoresForCgroups(cgResources.OfflineCgroups)
		if len(errs) != 0 {
			noError = false
			cpusetRecovered = true
		}
	}

	if noError {
		q.lastOfflineCgs.copy(cgResources.OfflineCgroups)
	} else {
		// clear record to handle next time
		q.lastNCores = nil
	}

	return nil
}

type cgroupPaths struct {
	cgroups []string
}

func newCgroupPaths() *cgroupPaths {
	return &cgroupPaths{
		cgroups: []string{},
	}
}

func (c *cgroupPaths) equal(cgs []string) bool {
	if len(cgs) != len(c.cgroups) {
		return false
	}

	allCgs := make(map[string]interface{})
	for _, cg := range cgs {
		allCgs[cg] = struct{}{}
	}

	for _, cg := range c.cgroups {
		if _, ok := allCgs[cg]; !ok {
			return false
		}
	}

	return true
}

func (c *cgroupPaths) copy(cgs []string) {
	newCgs := []string{}
	newCgs = append(newCgs, cgs...)
	c.cgroups = newCgs
}

func getOfflineCores(offlineRes v1.ResourceList) int {
	quantity, ok := offlineRes[v1.ResourceCPU]
	if !ok {
		klog.Warningf("no cpu resource found for isolation")
		return 0
	}

	return int(quantity.MilliValue() / 1000)
}

func setCoresForCgroups(cgs []string, cores []int, coresStr string) (errs []error) {
	for _, cgPath := range cgs {
		klog.V(4).Infof("writing cores:%v(%s) to cgroup %s", cores, coresStr, cgPath)
		var err error
		if len(coresStr) != 0 {
			err = cgroup.WriteCpuSetCoresStr(cgPath, coresStr)
		} else {
			err = cgroup.WriteCpuSetCores(cgPath, cores)
		}
		if err != nil {
			if len(coresStr) == 0 {
				coresStr = fmt.Sprintf("%v", cores)
			}
			klog.Errorf("write cores(%s) to cgroup %s err: %v",
				coresStr, cgPath, err)
			errs = append(errs, err)
		}
	}

	return errs
}

func recoverCoresForCgroups(cgs []string) (errs []error) {
	coresList, err := cgroup.GetCpuSet("", false)
	if err != nil {
		errs = append(errs, err)
		return
	}

	if len(coresList) != 1 {
		errs = append(errs, fmt.Errorf("invalid cpuset from root cgroup: %v", coresList))
		return
	}

	return setCoresForCgroups(cgs, []int{}, coresList[0])
}

func setSharesForCgroups(cgs []string, shares uint64) (errs []error) {
	for _, cg := range cgs {
		err := cgroup.SetCPUShares(cg, shares)
		if err != nil {
			klog.Errorf("set cpu shares value(%d) for cgroup(%s) err: %v", shares, cg, err)
			errs = append(errs, err)
		}
	}

	return errs
}

// initCgroup do some cgroup initializations
func initCgroup() error {
	if err := cgroup.EnsureCgroupPath(types.CgroupOffline); err != nil {
		return fmt.Errorf("create offline cgroup path(%s) err: %v", types.CgroupOffline, err)
	}

	if err := cgroup.EnsureCpuSetCores(types.CgroupOffline); err != nil {
		return fmt.Errorf("set cgroup(%s) cpuset cores err: %v", types.CgroupOffline, err)
	}

	if err := cgroup.GenerateTheadSiblings(); err != nil {
		return fmt.Errorf("generate numa thread silbings err: %v", err)
	}

	return nil
}

// parseCpuList constructs cpus from cpu list string, such as "0, 3-5"
func parseCpList(cpuList string) (sets.Int, error) {
	var cpus = sets.NewInt()

	if len(cpuList) == 0 {
		return cpus, nil
	}

	slices := strings.Split(cpuList, ",")
	for _, slice := range slices {
		if len(slice) == 1 {
			core, err := strconv.Atoi(slice)
			if err != nil {
				return cpus, fmt.Errorf("parse cpu(%s) to integer err: %v", slice, err)
			}
			cpus.Insert(core)
			continue
		}

		ranges := strings.Split(slice, "-")
		if len(ranges) != 2 {
			return cpus, fmt.Errorf("invalid cpu format: %s", slice)
		}
		start, errS := strconv.Atoi(ranges[0])
		end, errE := strconv.Atoi(ranges[1])
		if errS != nil || errE != nil {
			return cpus, fmt.Errorf("invalid cpu format(%s): %v, %v", ranges, errS, errE)
		}
		for i := start; i <= end; i++ {
			cpus.Insert(i)
		}
	}

	return cpus, nil
}
