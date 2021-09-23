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

package notify

import (
	"github.com/tencent/caelus/pkg/caelus/resource"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
	"k8s.io/klog"
)

// ResourceNotify describe resource monitor by notify method
type ResourceNotify interface {
	// Run main loop
	Run(stop <-chan struct{})
	Stop()
}

// notifier describe all kinds of resource notifier
type notifier interface {
	name() string
	start(stop <-chan struct{}) error
	stop() error
}

// notifyManager describe notify monitor data
// Now just support memory cgroup notify, need to add more
type notifyManager struct {
	notifiers []notifier
}

// NewNotifyManager creates notify manager instance, initializing all resource notifiers
func NewNotifyManager(cfg *types.NotifyConfig, nodeResource resource.Interface) ResourceNotify {
	var notifiers []notifier

	if cfg.MemoryCgroup != nil {
		memNotifier := newMemoryNotifier(cfg.MemoryCgroup, func() {
			nodeResource.KillOfflineJob(v1.ResourceMemory)
		})
		notifiers = append(notifiers, memNotifier)
	}

	return &notifyManager{
		notifiers: notifiers,
	}
}

// Run starts all resource notifier
func (n *notifyManager) Run(stop <-chan struct{}) {
	klog.V(2).Infof("cgroup notifier running")
	for _, nt := range n.notifiers {
		err := nt.start(stop)
		if err == nil {
			klog.Infof("notifier(%s) start successfully", nt.name())
		} else {
			klog.Errorf("notifier(%s) start failed: %v", nt.name(), err)
		}
	}
}

// Stop stops all resource notifier
func (n *notifyManager) Stop() {
	for _, nt := range n.notifiers {
		nt.stop()
	}
}
