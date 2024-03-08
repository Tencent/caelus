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

package rdtstore

import "C"
import (
	"fmt"
	"strings"
	"sync"
	"time"

	cgstore "github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/rdt/rdt"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	cadvisor "github.com/google/cadvisor/utils"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const rdtResourceName = "RDT"

// RdtMetrics show rdt metrics
type RdtMetrics struct {
	Spec  cgstore.CgroupRef
	Value rdt.RdtData
}

// RDTStoreInterface describe special interface for rdt resource state
type RDTStoreInterface interface {
	GetRDTResourceRangeStats(key string, start, end time.Time, count int) ([]*RdtMetrics, error)
	GetRDTResourceRecentState(key string) (*RdtMetrics, error)
	ListRDTResourceRangeStats(start, end time.Time, count int) (map[string][]*RdtMetrics, error)
	ListRDTResourceRecentStats() ([]*RdtMetrics, error)
}

// RDTStore describe common interface for rdt resource state inherit from state store
type RDTStore interface {
	RDTStoreInterface
	Name() string
	Run(stop <-chan struct{})
}

type rdtStoreManager struct {
	*types.MetricsRdtConfig
	maxAge   time.Duration
	lock     sync.RWMutex
	cpuLock  sync.Locker
	caches   map[string]*cadvisor.TimedStore
	cgroupSt cgstore.CgroupStoreInterface
}

// NewRdtStoreManager new rdt store manager
func NewRdtStoreManager(cacheTTL time.Duration, config *types.MetricsRdtConfig,
	cgStore cgstore.CgroupStoreInterface, cpuLock sync.Locker) RDTStore {
	rdt := &rdtStoreManager{
		caches:           make(map[string]*cadvisor.TimedStore),
		maxAge:           cacheTTL,
		MetricsRdtConfig: config,
		cgroupSt:         cgStore,
		cpuLock:          cpuLock,
	}

	return rdt
}

// Name show module name
func (r *rdtStoreManager) Name() string {
	return rdtResourceName
}

// Start calling main loop
func (r *rdtStoreManager) Run(stop <-chan struct{}) {
	if err := rdt.InitRdtCommand(r.RdtCommand); err != nil {
		klog.Fatalf("rdt command init failed: %v", err)
	}

	supported := rdt.SupportRdtFunction()
	if !supported.MonitorLLC {
		klog.Fatalf("rdt llc not supported")
	}

	go wait.Until(r.collect, r.CollectInterval.TimeDuration(), stop)
}

// GetRDTResourceRangeStats list RDT resource data in period by key
func (r *rdtStoreManager) GetRDTResourceRangeStats(key string,
	start, end time.Time, count int) ([]*RdtMetrics, error) {
	var metrics []*RdtMetrics
	var cache *cadvisor.TimedStore

	err := func() error {
		var ok bool
		r.lock.RLock()
		defer r.lock.RUnlock()

		cache, ok = r.caches[key]
		if !ok {
			return fmt.Errorf("rdt resource key %s not found", key)
		}
		return nil

	}()
	if err != nil {
		return metrics, err
	}

	items := cache.InTimeRange(start, end, count)
	for _, i := range items {
		metrics = append(metrics, i.(*RdtMetrics))
	}

	return metrics, nil
}

// GetRDTResourceRecentState get newest RDT data by key
func (r *rdtStoreManager) GetRDTResourceRecentState(key string) (*RdtMetrics, error) {
	var cache *cadvisor.TimedStore

	err := func() error {
		var ok bool
		r.lock.RLock()
		defer r.lock.RUnlock()

		cache, ok = r.caches[key]
		if !ok {
			return fmt.Errorf("rdt resource key %s not found", key)
		}
		return nil

	}()
	if err != nil {
		return nil, err
	}

	return cache.Get(0).(*RdtMetrics), nil

}

// ListRDTResourceRangeStats list all RDT data in period
func (r *rdtStoreManager) ListRDTResourceRangeStats(start, end time.Time,
	count int) (map[string][]*RdtMetrics, error) {
	sts := make(map[string][]*RdtMetrics)

	r.lock.RLock()
	defer r.lock.RUnlock()
	for k, cache := range r.caches {
		var metrics []*RdtMetrics
		values := cache.InTimeRange(start, end, count)
		for _, v := range values {
			metrics = append(metrics, v.(*RdtMetrics))
		}
		sts[k] = metrics
	}

	return sts, nil
}

// ListRDTResourceRecentStats list all newest RDT data
func (r *rdtStoreManager) ListRDTResourceRecentStats() ([]*RdtMetrics, error) {
	var metrics []*RdtMetrics

	r.lock.RLock()
	defer r.lock.RUnlock()
	for _, cache := range r.caches {
		value := cache.Get(0)
		metrics = append(metrics, value.(*RdtMetrics))
	}

	return metrics, nil
}

// collect main function to collect RDT data
func (r *rdtStoreManager) collect() {
	allCgroups, err := r.cgroupSt.ListAllCgroups(sets.NewString(appclass.AppClassOnline))
	if err != nil {
		return
	}

	r.cpuLock.Lock()
	defer r.cpuLock.Unlock()
	for k, v := range allCgroups {
		// just check pod with Guaranteed qos, which has static cores
		if strings.Contains(k, "burstable") {
			continue
		}
		// check pids
		pids, err := cgroup.GetCpuSetPids(k)
		if err != nil {
			klog.Errorf("cgroup(%s) get pid err: %v", k, err)
			continue
		}
		if len(pids) == 0 {
			klog.V(4).Infof("cgroup(%s) has no pid", k)
			continue
		}

		// read cpus
		cpus, err := cgroup.GetCpuSet(k, false)
		if err != nil {
			klog.Errorf("cgroup(%s) get cpu sets err: %v", k, err)
			continue
		}
		if len(cpus) == 0 {
			klog.Errorf("cgroup(%s) get cpu sets is nil", k)
			continue
		}

		// all cores together, no signal core
		monitors := []string{fmt.Sprintf("all:[%s]", cpus[0])}
		rets, err := rdt.StaticMonitor(monitors, r.ExecuteInterval.TimeDuration(), r.CollectDuration.TimeDuration())
		if err != nil {
			klog.Errorf("cgroup(%s) collect rdt data err: %v", k, err)
			continue
		}
		if len(rets) == 0 {
			klog.Errorf("cgroup(%s) collect no rdt data", k)
			continue
		}
		metric := &RdtMetrics{
			Spec:  *v,
			Value: rets[0],
		}
		r.addContainerRdt(k, rets[0].Timestamp, metric)
	}

	//rdt.MonitorReset()
	r.delContainerRdts()

	return
}

// addContainerRdt add RDT metrics into cache
func (r *rdtStoreManager) addContainerRdt(container string, now time.Time, stats interface{}) {
	var cache *cadvisor.TimedStore

	func() {
		var ok bool
		r.lock.Lock()
		defer r.lock.Unlock()

		cache, ok = r.caches[container]
		if !ok {
			cache = cadvisor.NewTimedStore(r.maxAge, -1)
			r.caches[container] = cache
		}
	}()

	cache.Add(now, stats)
}

// delContainerRdts delete old container RDT data
func (r *rdtStoreManager) delContainerRdts() {
	r.lock.Lock()
	defer r.lock.Unlock()

	for c, cache := range r.caches {
		// get last element
		stats := cache.InTimeRange(time.Now().Add(-10*time.Minute), time.Now(), -1)
		if len(stats) == 0 {
			klog.Warningf("delete container(%s) rdt cache for time expired", c)
			delete(r.caches, c)
		}
	}
}
