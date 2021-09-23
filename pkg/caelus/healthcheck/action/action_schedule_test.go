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

package action

import "testing"

// TestScheduleAction_DoAction test how schedule action working
func TestScheduleAction_DoAction(t *testing.T) {
	actionName := "testing"
	describe := "schedule action testing"
	scheduleDisabled := true

	scheduleAction := NewScheduleAction(actionName)
	acRet, err := scheduleAction.DoAction(scheduleDisabled, "just testing")
	if err != nil {
		t.Fatalf("%s: DoAction return err: %v", describe, err)
	}

	schState, ok := acRet.UnscheduleMap[actionName]
	if !ok {
		t.Fatalf("%s: key(%s) not found in UnscheduleMap: %v", describe, actionName, acRet.UnscheduleMap)
	}
	if schState != scheduleDisabled {
		t.Fatalf("%s: schedule state is %v, expect %v", describe, schState, scheduleDisabled)
	}
}
