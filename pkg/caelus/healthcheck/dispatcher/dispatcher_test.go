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
	"testing"

	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict/mock"
	"github.com/tencent/caelus/pkg/caelus/qos/mock"
	"github.com/tencent/caelus/pkg/caelus/resource"

	"k8s.io/api/core/v1"
)

var dispatcherHandleCases = []struct {
	describe                string
	existingUnscheduleState bool
	acRet                   *action.ActionResult
	expectUnschedule        bool
	expectNotify            bool
}{
	{
		describe:                "existing unSchedule state is false",
		existingUnscheduleState: false,
		acRet: &action.ActionResult{
			UnscheduleMap:   map[string]bool{string(v1.ResourceCPU): true},
			AdjustResources: map[v1.ResourceName]action.ActionResource{},
		},
		expectUnschedule: true,
		expectNotify:     false,
	},
	{
		describe:                "existing unSchedule state is true",
		existingUnscheduleState: true,
		acRet: &action.ActionResult{
			UnscheduleMap:   map[string]bool{string(v1.ResourceCPU): false},
			AdjustResources: map[v1.ResourceName]action.ActionResource{},
		},
		expectUnschedule: false,
		expectNotify:     false,
	},
	{
		describe:                "update conflicting resource",
		existingUnscheduleState: true,
		acRet: &action.ActionResult{
			UnscheduleMap:   map[string]bool{string(v1.ResourceCPU): false},
			AdjustResources: map[v1.ResourceName]action.ActionResource{v1.ResourceCPU: {}},
		},
		expectUnschedule: false,
		expectNotify:     true,
	},
	{
		describe:                "notify event",
		existingUnscheduleState: false,
		acRet: &action.ActionResult{
			UnscheduleMap:   map[string]bool{string(v1.ResourceCPU): false},
			AdjustResources: map[v1.ResourceName]action.ActionResource{},
			SyncEvent:       true,
		},
		expectUnschedule: false,
		expectNotify:     true,
	},
}

// TestDispatcher_HandleActionResult test dispatch action result
func TestDispatcher_HandleActionResult(t *testing.T) {
	for _, tCase := range dispatcherHandleCases {
		t.Logf("dispatch conflicting state testing: %s", tCase.describe)
		resManager := resource.NewMockResource()
		qosManager := qos.NewMockQOS()
		conflictManager := conflict.NewMockConflict(map[v1.ResourceName]action.ActionResource{})

		dispatchManager := NewDispatcher(resManager, qosManager, conflictManager)
		dispatchManager.unschedule = map[string]bool{string(v1.ResourceCPU): tCase.existingUnscheduleState}
		err := dispatchManager.HandleActionResult(tCase.acRet)
		if err != nil {
			t.Fatalf("dispatch conflicting state case(%s) err: %v", tCase.describe, err)
		}

		if resManager.(*resource.MockResource).ScheduledDisabled != tCase.expectUnschedule {
			t.Fatalf("dispatch conflicting state case(%s) got unexpected schedule state, should be %v",
				tCase.describe, tCase.expectUnschedule)
		}

		if qosManager.(*qos.MockQOS).ReceivedEvent != tCase.expectNotify {
			t.Fatalf("dispatch conflicting state case(%s) got unexpected notify state, should be %v",
				tCase.describe, tCase.expectNotify)
		}
	}
}
