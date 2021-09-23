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

package statestore

import (
	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/statestore/common"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// StateStore describe interface for resource state
type StateStore interface {
	Run(stop <-chan struct{})
	Name() string
	commonstore.CommonStoreInterface
	cgroupstore.CgroupStoreInterface
}

type stateStoreManager struct {
	cgroupstore.CgroupStore
	commonstore.CommonStore
}

// NewStateStoreManager new a resource state instance
func NewStateStoreManager(config *types.MetricsCollectConfig, onlineCfg *types.OnlineConfig,
	podInformer cache.SharedIndexInformer) StateStore {
	cgroupSt := cgroupstore.NewCgroupStoreManager(config.Container, podInformer)
	publicSt := commonstore.NewCommonStoreManager(config, onlineCfg, podInformer, cgroupSt)
	return &stateStoreManager{
		CgroupStore: cgroupSt,
		CommonStore: publicSt,
	}
}

// Run main loop
func (ss *stateStoreManager) Run(stop <-chan struct{}) {
	ss.CommonStore.Run(stop)
	klog.V(2).Infof("public resource state collection started")
	ss.CgroupStore.Run(stop)
	klog.V(2).Infof("cgroup resource state collection started")
}

// Name describe module name
func (ss *stateStoreManager) Name() string {
	return "ModuleStateStore"
}
