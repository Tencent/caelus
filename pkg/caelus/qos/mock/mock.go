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

package qos

import (
	"github.com/tencent/caelus/pkg/caelus/qos"
	"github.com/tencent/caelus/pkg/caelus/types"
)

// MockQOS mock QOS manager
type MockQOS struct {
	ReceivedEvent bool
}

// NewMockQOS new a mock QOS manager
func NewMockQOS() qos.Manager {
	return &MockQOS{}
}

// Name mock function
func (m *MockQOS) Name() string {
	return "mockQOS"
}

// Run mock function
func (m *MockQOS) Run(stop <-chan struct{}) {
	panic("implement me")
}

// UpdateEvent mock function
func (m *MockQOS) UpdateEvent(event *types.ResourceUpdateEvent) error {
	m.ReceivedEvent = true
	return nil
}
