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

package perfstore

import (
	"fmt"
	"strings"
	"sync"
	"time"

	cgstore "github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/perf/pmu"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	cadvisor "github.com/google/cadvisor/utils"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const perfResourceName = "Perf"

// PerfMetrics show perf date for the cgroup
type PerfMetrics struct {
	Spec  cgstore.CgroupRef
	Value pmu.PerfData
}

// PerfStoreInterface describe perf special interface
type PerfStoreInterface interface {
	GetPerfResourceRangeStats(key string, start, end time.Time, count int) ([]*PerfMetrics, error)
	GetPerfResourceRecentState(key string) (*PerfMetrics, error)
	ListPerfResourceRangeStats(start, end time.Time, count int) (map[string][]*PerfMetrics, error)
	ListPerfResourceRecentStats() ([]*PerfMetrics, error)
}

// PerfStore describe common interface for perf resource inherit from state store
type PerfStore interface {
	PerfStoreInterface
	Name() string
	Run(stop <-chan struct{})
}

type perfStoreManager struct {
	*types.MetricsPerfConfig
	lock     sync.RWMutex
	cpuLock  sync.Locker
	caches   map[string]*cadvisor.TimedStore
	maxAge   time.Duration
	cgroupSt cgstore.CgroupStoreInterface
}

// NewPerfStoreManager new perf store manager
func NewPerfStoreManager(cacheTTL time.Duration, config *types.MetricsPerfConfig,
	cgroupSt cgstore.CgroupStoreInterface, cpuLock sync.Locker) PerfStore {
	return &perfStoreManager{
		MetricsPerfConfig: config,
		caches:            make(map[string]*cadvisor.TimedStore),
		maxAge:            cacheTTL,
		cgroupSt:          cgroupSt,
		cpuLock:           cpuLock,
	}
}

// Name show module name
func (p *perfStoreManager) Name() string {
	return perfResourceName
}

// Start call main loop
func (p *perfStoreManager) Run(stop <-chan struct{}) {
	go wait.Until(p.collect, p.CollectInterval.TimeDuration(), stop)
}

// GetPerfResourceRangeStats list perf data in period time by key
func (p *perfStoreManager) GetPerfResourceRangeStats(key string,
	start, end time.Time, count int) ([]*PerfMetrics, error) {
	var metrics []*PerfMetrics
	var cache *cadvisor.TimedStore

	err := func() error {
		var ok bool
		p.lock.RLock()
		defer p.lock.RUnlock()

		cache, ok = p.caches[key]
		if !ok {
			return fmt.Errorf("perf resource key %s not found", key)
		}
		return nil

	}()
	if err != nil {
		return metrics, err
	}

	items := cache.InTimeRange(start, end, count)
	for _, i := range items {
		metrics = append(metrics, i.(*PerfMetrics))
	}

	return metrics, nil
}

// GetPerfResourceRecentState get newest perf data by key
func (p *perfStoreManager) GetPerfResourceRecentState(key string) (*PerfMetrics, error) {
	var cache *cadvisor.TimedStore

	err := func() error {
		var ok bool
		p.lock.RLock()
		defer p.lock.RUnlock()

		cache, ok = p.caches[key]
		if !ok {
			return fmt.Errorf("perf resource key %s not found", key)
		}
		return nil

	}()
	if err != nil {
		return nil, err
	}

	return cache.Get(0).(*PerfMetrics), nil

}

// ListPerfResourceRangeStats list all perf data in period
func (p *perfStoreManager) ListPerfResourceRangeStats(start, end time.Time,
	count int) (map[string][]*PerfMetrics, error) {
	sts := make(map[string][]*PerfMetrics)

	p.lock.RLock()
	defer p.lock.RUnlock()
	for k, cache := range p.caches {
		var metrics []*PerfMetrics
		values := cache.InTimeRange(start, end, count)
		for _, v := range values {
			metrics = append(metrics, v.(*PerfMetrics))
		}
		sts[k] = metrics
	}

	return sts, nil
}

// ListPerfResourceRecentStats list all newest perf data
func (p *perfStoreManager) ListPerfResourceRecentStats() ([]*PerfMetrics, error) {
	var metrics []*PerfMetrics

	p.lock.RLock()
	defer p.lock.RUnlock()
	for _, cache := range p.caches {
		value := cache.Get(0)
		metrics = append(metrics, value.(*PerfMetrics))
	}

	return metrics, nil
}

// collect collect perf metrics
func (p *perfStoreManager) collect() {
	allCgroups, err := p.cgroupSt.ListAllCgroups(sets.NewString(appclass.AppClassOnline))
	if err != nil {
		return
	}

	p.cpuLock.Lock()
	defer p.cpuLock.Unlock()

	wg := sync.WaitGroup{}
	for k, v := range allCgroups {

		ignoreFlag := false

		for _, ignored := range p.IgnoredCgroups {
			if checkSubCgroup(ignored, k) {
				klog.V(4).Infof("cgroup(%s) has been ignored", k)
				ignoreFlag = true
				break
			}
		}
		if ignoreFlag {
			continue
		}

		wg.Add(1)
		go func(cg string, ref *cgstore.CgroupRef) {
			defer wg.Done()

			cgPath, err := cgroup.GetPerfEventCgroupPath(cg)
			if err != nil {
				klog.Errorf("get perf_event cgroup path err: %v", err)
				return
			}
			// check pids
			pids, err := cgroup.GetPids(cgPath)
			if err != nil {
				klog.Errorf("cgroup(%s) get pid err: %v", cg, err)
				return
			}
			if len(pids) == 0 {
				klog.V(4).Infof("cgroup(%s) has no pid", cg)
				return
			}

			// read cpus
			cpus, err := cgroup.GetCpuSet(cg, true)
			if err != nil {
				klog.Errorf("cgroup(%s) get cpu sets err: %v", cg, err)
				return
			}
			if len(cpus) == 0 {
				klog.Errorf("cgroup(%s) get cpu sets is nil", cg)
				return
			}

			start := time.Now()
			cpuStartTotal, err := cgroup.GetCPUTotalUsage(cg)
			if err != nil {
				klog.Errorf("cgroup(%s) collect cpu usage failed: %v", cg, err)
				return
			}
			pmuData, err := pmu.GetPMUValue(int(p.CollectDuration.Seconds()),
				cgPath, strings.Join(cpus, ","))
			if err != nil {
				klog.Errorf("cgroup(%s) collect perf data err: %v", cg, err)
				return
			}
			timeElapsed := time.Since(start).Nanoseconds()
			cpuEndTotal, err := cgroup.GetCPUTotalUsage(cg)
			if err != nil {
				klog.Errorf("cgroup(%s) collect cpu usage failed: %v", cg, err)
				return
			}
			pmuData.CPUUsage = float64(cpuEndTotal-cpuStartTotal) / float64(timeElapsed)

			metric := &PerfMetrics{
				Spec:  *ref,
				Value: pmuData,
			}
			p.addContainerPerf(cg, pmuData.Timestamp, metric)
		}(k, v)
	}
	wg.Wait()

	p.delContainerPerfs()

	return
}

// addContainerPerf add container perf data into cache
func (p *perfStoreManager) addContainerPerf(container string, now time.Time, stats interface{}) {
	var cache *cadvisor.TimedStore

	func() {
		var ok bool
		p.lock.Lock()
		defer p.lock.Unlock()

		cache, ok = p.caches[container]
		if !ok {
			cache = cadvisor.NewTimedStore(p.maxAge, -1)
			p.caches[container] = cache
		}
	}()

	cache.Add(now, stats)
}

// delContainerPerfs delete container perf data
func (p *perfStoreManager) delContainerPerfs() {
	p.lock.Lock()
	defer p.lock.Unlock()

	for c, cache := range p.caches {
		// get last elements
		stats := cache.InTimeRange(time.Now().Add(-10*time.Minute), time.Now(), -1)
		if len(stats) == 0 {
			klog.Warningf("delete container(%s) perf cache for time expired", c)
			delete(p.caches, c)
		}
	}
}

// checkSubCgroup check if the two cgroups are father-son relation,
// like /kubepods/besteffort and /kubepods/besteffort/aaa
func checkSubCgroup(parent, child string) bool {
	if strings.HasPrefix(child, parent) {
		return true
	}
	return false
}
