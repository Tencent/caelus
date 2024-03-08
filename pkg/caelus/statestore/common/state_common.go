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

package commonstore

import (
	"github.com/tencent/caelus/pkg/caelus/statestore/common/customize"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/prometheus"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/node"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/perf"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/rdt"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

var _ CommonStore = &commonStoreManager{}

// CommonStoreInterface gather all resource store interface
type CommonStoreInterface interface {
	nodestore.NodeStoreInterface
	perfstore.PerfStoreInterface
	rdtstore.RDTStoreInterface
	customizestore.CustomizeStoreInterface
	promstore.PromStoreInterface
}

// CommonStore describe functions for resource store interface
type CommonStore interface {
	Run(stop <-chan struct{})
	CommonStoreInterface
}

type runHandler struct {
	name    string
	handler func(stop <-chan struct{})
}

type commonStoreManager struct {
	nodestore.NodeStore
	perfstore.PerfStore
	rdtstore.RDTStore
	customizestore.CustomizeStore
	promstore.PromStore

	runHandlers []runHandler
}

// NewCommonStoreManager new resource common state interface
func NewCommonStoreManager(config *types.MetricsCollectConfig, onlineCfg *types.OnlineConfig,
	podInformer cache.SharedIndexInformer, cgroupSt cgroupstore.CgroupStoreInterface) CommonStore {
	// cpuLock is used to lock collecting PMU and RDT, which could not be running together
	var cpuLock sync.Mutex
	var runFuncs []runHandler
	var cacheAge = time.Duration(10 * time.Minute)

	// append node resource state
	nodeSt := nodestore.NewNodeStoreManager(cacheAge, &config.Node, podInformer)
	runFuncs = append(runFuncs, runHandler{name: nodeSt.Name(), handler: nodeSt.Run})

	// append perf resource state
	perfSt := perfstore.NewPerfStoreManager(cacheAge, &config.Perf, cgroupSt, &cpuLock)
	if !config.Perf.Disable {
		runFuncs = append(runFuncs, runHandler{name: perfSt.Name(), handler: perfSt.Run})
	} else {
		klog.Warningf("collecting %s resources is disabled", perfSt.Name())
	}

	// append RDT resource state
	rdtSt := rdtstore.NewRdtStoreManager(cacheAge, &config.Rdt, cgroupSt, &cpuLock)
	if !config.Rdt.Disable {
		runFuncs = append(runFuncs, runHandler{name: rdtSt.Name(), handler: rdtSt.Run})
	} else {
		klog.Warningf("collecting %s resources is disabled", rdtSt.Name())
	}

	// append prometheus resource state
	promSt := promstore.NewPromStoreManager(cacheAge, &config.Prometheus)
	runFuncs = append(runFuncs, runHandler{name: promSt.Name(), handler: promSt.Run})

	// append app metrics resource state
	customizeSt := customizestore.NewCustomizeStoreManager(cacheAge, onlineCfg)
	runFuncs = append(runFuncs, runHandler{name: customizeSt.Name(), handler: customizeSt.Run})

	return &commonStoreManager{
		NodeStore:      nodeSt,
		PerfStore:      perfSt,
		RDTStore:       rdtSt,
		PromStore:      promSt,
		CustomizeStore: customizeSt,
		runHandlers:    runFuncs,
	}
}

// Run main loop
func (ps *commonStoreManager) Run(stop <-chan struct{}) {
	for _, runFunc := range ps.runHandlers {
		klog.V(2).Infof("start collecting %s resources", runFunc.name)
		runFunc.handler(stop)
	}
}
