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

package yarn

import (
	"github.com/tencent/caelus/pkg/caelus/types"
	global "github.com/tencent/caelus/pkg/types"

	"k8s.io/api/core/v1"
)

type fakeGinit struct {
	minCap *global.NMCapacity
}

func newFakeGinit(minCap *global.NMCapacity) GInitInterface {
	return &fakeGinit{
		minCap: minCap,
	}
}

// GetStatus check if node manager container is ready
func (fg *fakeGinit) GetStatus() (bool, error) {
	panic("implement me")
}

// StartNodemanager start node manager
func (fg *fakeGinit) StartNodemanager() error {
	panic("implement me")
}

// StopNodemanager stop node manager
func (fg *fakeGinit) StopNodemanager() error {
	panic("implement me")
}

// GetProperty get property map based on keys
func (fg *fakeGinit) GetProperty(fileName string, keys []string, allKeys bool) (map[string]string, error) {
	panic("implement me")
}

// SetProperty set property value
func (fg *fakeGinit) SetProperty(fileName string, properties map[string]string,
	addNewKeys, delNonExistedKeys bool) error {
	panic("implement me")
}

// GetCapacity return nodemanager resource capacity
func (fg *fakeGinit) GetCapacity() (*global.NMCapacity, error) {
	panic("implement me")
}

// SetCapacity set nodemanager resource capacity
func (fg *fakeGinit) SetCapacity(capacity *global.NMCapacity) error {
	panic("implement me")
}

// EnsureCapacity set capacity resource, and kill containers if necessary
func (fg *fakeGinit) EnsureCapacity(expect *global.NMCapacity, conflictingResources []string, decreaseCap, scheduleDisabled bool) {
	panic("implement me")
}

// WatchForMetricsPort watch changes of nodemanager metrics port, it is not thread safe
func (fg *fakeGinit) WatchForMetricsPort() chan int {
	panic("implement me")
}

// GetNMWebappPort get nodemanager webapp port
func (fg *fakeGinit) GetNMWebappPort() (int, error) {
	panic("implement me")
}

// GetAllocatedJobs returns containers list, including resources and job state
func (fg *fakeGinit) GetAllocatedJobs() ([]types.OfflineJobs, error) {
	panic("implement me")
}

// GetMinCapacity get minimum capacity for nodemanager
func (fg *fakeGinit) GetMinCapacity() *global.NMCapacity {
	return fg.minCap
}

// KillContainer kill at least one container
func (fg *fakeGinit) KillContainer(conflictingResource v1.ResourceName) {
	panic("implement me")
}

// DisableSchedule disable nodemanager accepting new jobs
func (fg *fakeGinit) DisableSchedule() error {
	panic("implement me")
}

// EnableSchedule recover nodemanager accepting new jobs
func (fg *fakeGinit) EnableSchedule() error {
	panic("implement me")
}

// UpdateNodeCapacity update nodemanager capacity,
// Force updating when the node is in schedule disabled state if the force parameter is true
func (fg *fakeGinit) UpdateNodeCapacity(capacity *global.NMCapacity, force bool) error {
	panic("implement me")
}
