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

package rulecheck

import (
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/dispatcher"
	"github.com/tencent/caelus/pkg/caelus/qos"
	"github.com/tencent/caelus/pkg/caelus/resource"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// ruleChecker defines interface for rule checker
type ruleChecker interface {
	Name() string
	Check() *action.ActionResult
}

// Manager describe options for rule checking
type Manager struct {
	checkInterval time.Duration
	checkerLock   sync.Mutex
	checkers      []ruleChecker
	dispatcher    *dispatcher.Dispatcher

	stopCh chan struct{}
}

// NewManager return a new rule check manager
func NewManager(config types.RuleCheck, stStore statestore.StateStore, resource resource.Interface,
	qosManager qos.Manager, conflictMn conflict.Manager, podInformer cache.SharedIndexInformer,
	predictReserved *types.Resource) *Manager {
	var checkers []ruleChecker
	checkers = append(checkers, newContainerHealthChecker(stStore, podInformer, config.ContainerRules))
	checkers = append(checkers, newNodeHealthChecker(stStore, predictReserved, config.NodeRules))
	checkers = append(checkers, newAppHealthChecker(stStore, config.AppRules))

	return &Manager{
		// default global loop check interval
		checkInterval: 1 * time.Second,
		checkers:      checkers,
		dispatcher:    dispatcher.NewDispatcher(resource, qosManager, conflictMn),

		stopCh: make(chan struct{}),
	}
}

// Run start checking rules periodically
func (r *Manager) Run(stopCh <-chan struct{}) {
	klog.V(2).Infof("health check running")
	for {
		select {
		case <-stopCh:
			return
		case <-r.stopCh:
			return
		default:
		}
		wg := sync.WaitGroup{}
		finalAC := &action.ActionResult{
			UnscheduleMap:   make(map[string]bool),
			AdjustResources: make(map[v1.ResourceName]action.ActionResource),
		}

		// run different checking in parallel, and wait until all finished
		for i := range r.checkers {
			checker := r.checkers[i]
			wg.Add(1)
			go func() {
				ac := checker.Check()
				r.checkerLock.Lock()
				finalAC.Merge(ac)
				r.checkerLock.Unlock()

				wg.Done()
			}()
		}
		wg.Wait()

		// no need dispatch result when action is useless
		if actionResultAvailable(finalAC) {
			klog.V(3).Infof("finding action handle: %+v", finalAC)
			err := r.dispatcher.HandleActionResult(finalAC)
			if err != nil {
				klog.Errorf("handle action result(%v) err: %v", finalAC, err)
			}
		}

		time.Sleep(r.checkInterval)
	}
}

// Stop stop current manager
func (r *Manager) Stop() {
	close(r.stopCh)
}

// actionResultAvailable check if the action result is available
func actionResultAvailable(ac *action.ActionResult) bool {
	if len(ac.AdjustResources) > 0 || len(ac.EvictPods) > 0 {
		return true
	}
	if len(ac.UnscheduleMap) != 0 || ac.SyncEvent {
		return true
	}

	return false
}
