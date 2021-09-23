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
	"github.com/tencent/caelus/pkg/caelus/statestore"
)

// MockStatStore describe fake resource stats type
type MockStatStore struct {
	*MockCommonStore
	*MockCgroupStore
}

// NewMockStatStore create fake resource stats instance
func NewMockStatStore(common *MockCommonStore, cgroup *MockCgroupStore) statestore.StateStore {
	return &MockStatStore{
		MockCommonStore: common,
		MockCgroupStore: cgroup,
	}
}

// mock function
func (ss *MockStatStore) Run(stopCh <-chan struct{}) {
	panic("implement me")
}

// mock function
func (ss *MockStatStore) Name() string {
	panic("implement me")
}
