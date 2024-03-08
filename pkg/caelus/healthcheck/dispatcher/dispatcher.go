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

package dispatcher

import (
	"strings"

	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/qos"
	"github.com/tencent/caelus/pkg/caelus/resource"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

// Dispatcher used to send rule checking result to related modules
type Dispatcher struct {
	resourceManager resource.Interface
	qosManager      qos.Manager
	conflictMn      conflict.Manager

	unschedule map[string]bool
}

// NewDispatcher create a new dispatcher instance
func NewDispatcher(resourceManager resource.Interface, qosManager qos.Manager,
	conflictMn conflict.Manager) *Dispatcher {
	return &Dispatcher{
		resourceManager: resourceManager,
		qosManager:      qosManager,
		conflictMn:      conflictMn,
		unschedule:      make(map[string]bool),
	}
}

// HandleActionResult dispatch rule checking result to all related modules
func (d *Dispatcher) HandleActionResult(acRet *action.ActionResult) error {
	var err error
	for k, v := range acRet.UnscheduleMap {
		d.unschedule[k] = v
	}
	unschedule := false
	for _, un := range d.unschedule {
		unschedule = unschedule || un
	}

	if d.resourceManager != nil {
		if unschedule {
			err = d.resourceManager.DisableOfflineSchedule()
			if err != nil {
				// just warning
				klog.Errorf("disable offline schedule err: %v", err)
			}
		} else {
			err = d.resourceManager.EnableOfflineSchedule()
			if err != nil {
				// just warning
				klog.Errorf("enable offline schedule err: %v", err)
			}
		}
	}

	changed, err := d.conflictMn.UpdateConflictList(acRet.AdjustResources)
	if err != nil {
		klog.Errorf("update conflicting resource list err: %v", err)
		return err
	}
	//var conflictRes v1.ResourceName = ""
	conflictRes := sets.NewString()
	for _, ac := range acRet.AdjustResources {
		if ac.Conflicting {
			conflictRes.Insert(string(ac.Name))
		}
	}

	if acRet.SyncEvent || changed {
		event := &types.ResourceUpdateEvent{
			ConflictRes: conflictRes.List(),
			Reason:      strings.Join(acRet.Messages, ";"),
		}
		if d.resourceManager != nil {
			d.resourceManager.SyncNodeResource(event)
		}
		if d.qosManager != nil {
			d.qosManager.UpdateEvent(event)
		}
	}

	// TODO evict pods

	return nil
}
