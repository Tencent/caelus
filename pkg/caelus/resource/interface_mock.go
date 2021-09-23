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

package resource

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
)

// MockResource mock Resource manager
type MockResource struct {
	ReceivedEvent     bool
	ScheduledDisabled bool
}

// NewMockResource new a mock Resource manager
func NewMockResource() Interface {
	return &MockResource{}
}

// Name mock function
func (m *MockResource) Name() string {
	return "MockResource"
}

// Run mock function
func (m *MockResource) Run(stopCh <-chan struct{}) {
	panic("implementing me")
}

// DisableOfflineSchedule mock function
func (m *MockResource) DisableOfflineSchedule() error {
	m.ScheduledDisabled = true
	return nil
}

// EnableOfflineSchedule mock function
func (m *MockResource) EnableOfflineSchedule() error {
	m.ScheduledDisabled = false
	return nil
}

// GetOfflineJobs mock function
func (m *MockResource) GetOfflineJobs() ([]types.OfflineJobs, error) {
	panic("implementing me")
}

// KillOfflineJob mock function
func (m *MockResource) KillOfflineJob(conflictingResource v1.ResourceName) {
	panic("implementing me")
}

// Describe mock function
func (m *MockResource) Describe(chan<- *prometheus.Desc) {
	panic("implementing me")
}

// Collect mock function
func (m *MockResource) Collect(chan<- prometheus.Metric) {
	panic("implementing me")
}

// SyncNodeResource mock function
func (m *MockResource) SyncNodeResource(event *types.ResourceUpdateEvent) error {
	m.ReceivedEvent = true
	return nil
}

// mockClientInterface mock resource client
type mockClientInterface struct {
	*MockResource
}

// newMockClientInterface create a resource mock client
func newMockClientInterface() clientInterface {
	return &mockClientInterface{
		MockResource: &MockResource{},
	}
}

// Init need to implement
func (m *mockClientInterface) Init() error {
	panic("implementing me")
}

// CheckPoint need to implement
func (m *mockClientInterface) CheckPoint() error {
	panic("implementing me")
}

// AdaptAndUpdateOfflineResource need to implement
func (m *mockClientInterface) AdaptAndUpdateOfflineResource(offlineList v1.ResourceList,
	conflictingResources []string) error {
	panic("implementing me")
}
