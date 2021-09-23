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

package mock

import (
	"fmt"
	"time"

	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ cgroupstore.CgroupStoreInterface = &MockCgroupStore{}

// MockCgroupStore mock cgroup resource store.
// Functions are now panic if not used in testing, and you need to implement the codes when using.
type MockCgroupStore struct {
	CgStats  map[string]*cgroupstore.CgroupStats
	ExtraCgs []string
}

// mock function
func (cg *MockCgroupStore) MachineInfo() (*cadvisorapi.MachineInfo, error) {
	panic("implement me")
}

// mock function
func (cg *MockCgroupStore) GetCgroupResourceRangeStats(podName, podNamespace string, start, end time.Time,
	count int) ([]*cgroupstore.CgroupStats, error) {
	panic("implement me")
}

// mock function
func (cg *MockCgroupStore) GetCgroupResourceRecentState(podName, podNamespace string,
	updateStats bool) (*cgroupstore.CgroupStats, error) {
	panic("implement me")

}

// mock function
func (cg *MockCgroupStore) GetCgroupResourceRecentStateByPath(cgPath string,
	updateStats bool) (*cgroupstore.CgroupStats, error) {
	for _, cgStat := range cg.CgStats {
		if cgStat.Ref.Name == cgPath {
			return cgStat, nil
		}
	}
	return nil, fmt.Errorf("no cgroup stats found for path: %s", cgPath)
}

// mock function
func (cg *MockCgroupStore) ListCgroupResourceRangeStats(start, end time.Time, count int,
	classFilter sets.String) (map[string][]*cgroupstore.CgroupStats, error) {
	panic("implement me")
}

// mock function
func (cg *MockCgroupStore) ListCgroupResourceRecentState(updateStats bool,
	classFilter sets.String) (map[string]*cgroupstore.CgroupStats, error) {
	return cg.CgStats, nil
}

// mock function
func (cg *MockCgroupStore) ListAllCgroups(classFilter sets.String) (map[string]*cgroupstore.CgroupRef, error) {
	panic("implement me")
}

// mock function
func (cg *MockCgroupStore) AddExtraCgroups(extraCgs []string) error {
	cg.ExtraCgs = extraCgs
	return nil
}

// mock function
func (cg *MockCgroupStore) GetCgroupStoreSupportedTags() []string {
	panic("implement me")
}
