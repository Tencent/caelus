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

package conflict

import (
	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"

	"k8s.io/api/core/v1"
)

// MockConflict mock conflict manager
type MockConflict struct {
	conflictingResources map[v1.ResourceName]action.ActionResource
}

// NewMockConflict new a conflict mock manager
func NewMockConflict(res map[v1.ResourceName]action.ActionResource) conflict.Manager {
	return &MockConflict{conflictingResources: res}
}

// CheckAndSubConflictResource mock function
func (m *MockConflict) CheckAndSubConflictResource(predictList v1.ResourceList) (map[v1.ResourceName]bool, error) {
	panic("implement me")
}

// UpdateConflictList mock function
func (m *MockConflict) UpdateConflictList(conflictList map[v1.ResourceName]action.ActionResource) (bool, error) {
	for k, v := range conflictList {
		vv, ok := m.conflictingResources[k]
		if !ok {
			return true, nil
		}

		if !v.Equal(&vv) {
			return true, nil
		}
	}

	return false, nil
}
