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

import (
	"k8s.io/api/core/v1"
)

type scheduleAction struct {
	name string
}

// NewScheduleAction get Action instance to unschedule offline job
func NewScheduleAction(name string) Action {
	return &scheduleAction{name: name}
}

// ActionType return current action type
func (s *scheduleAction) ActionType() ActionType {
	return Schedule
}

// DoAction handle check result
func (s *scheduleAction) DoAction(conflicting bool, data interface{}) (*ActionResult, error) {
	var ac = &ActionResult{
		UnscheduleMap:   map[string]bool{s.name: conflicting},
		AdjustResources: make(map[v1.ResourceName]ActionResource),
		Messages:        []string{data.(string)},
	}

	return ac, nil
}
