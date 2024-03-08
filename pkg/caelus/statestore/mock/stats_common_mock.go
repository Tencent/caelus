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

	"github.com/tencent/caelus/pkg/caelus/statestore/common"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/customize"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/node"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/perf"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/prometheus"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/rdt"
	"github.com/tencent/caelus/pkg/caelus/types"

	dto "github.com/prometheus/client_model/go"
	"k8s.io/klog/v2"
)

var _ commonstore.CommonStoreInterface = &MockCommonStore{}
var _ nodestore.NodeStoreInterface = &MockNodeStore{}
var _ perfstore.PerfStoreInterface = &MockPerfStore{}
var _ rdtstore.RDTStoreInterface = &MockRDTStore{}
var _ promstore.PromStoreInterface = &MockPromStore{}

// MockCommonStore mock common resource stat store
type MockCommonStore struct {
	*MockNodeStore
	*MockPerfStore
	*MockRDTStore
	*MockCustomizeStore
	*MockPromStore
}

// MockNodeStore mock node resource store
type MockNodeStore struct {
	Resources []*nodestore.NodeResourceState
}

// mock function
func (n *MockNodeStore) GetNodeResourceRangeStats(start, end time.Time, count int) (
	[]*nodestore.NodeResourceState, error) {
	panic("implement me")
}

// mock function
func (n *MockNodeStore) GetNodeResourceRecentState() (*nodestore.NodeResourceState, error) {
	if len(n.Resources) > 0 {
		return n.Resources[len(n.Resources)-1], nil
	} else {
		return nil, fmt.Errorf("no data")
	}
}

// mock function
func (n *MockNodeStore) GetNodeStoreSupportedTags() []string {
	panic("implement me")
}

// MockPerfStore mock perf resource store
type MockPerfStore struct{}

// mock function
func (p *MockPerfStore) GetPerfResourceRangeStats(key string, start, end time.Time,
	count int) ([]*perfstore.PerfMetrics, error) {
	panic("implement me")
}

// mock function
func (p *MockPerfStore) GetPerfResourceRecentState(key string) (*perfstore.PerfMetrics, error) {
	panic("implement me")
}

// mock function
func (p *MockPerfStore) ListPerfResourceRangeStats(start, end time.Time,
	count int) (map[string][]*perfstore.PerfMetrics, error) {
	panic("implement me")
}

// mock function
func (p *MockPerfStore) ListPerfResourceRecentStats() ([]*perfstore.PerfMetrics, error) {
	panic("implement me")
}

// MockRDTStore mock rdt resource store
type MockRDTStore struct{}

// mock function
func (r *MockRDTStore) GetRDTResourceRangeStats(key string, start, end time.Time,
	count int) ([]*rdtstore.RdtMetrics, error) {
	panic("implement me")
}

// mock function
func (r *MockRDTStore) GetRDTResourceRecentState(key string) (*rdtstore.RdtMetrics, error) {
	panic("implement me")
}

// mock function
func (r *MockRDTStore) ListRDTResourceRangeStats(start, end time.Time,
	count int) (map[string][]*rdtstore.RdtMetrics, error) {
	panic("implement me")
}

// mock function
func (r *MockRDTStore) ListRDTResourceRecentStats() ([]*rdtstore.RdtMetrics, error) {
	panic("implement me")
}

// MockCustomizeStore mock customize resource store
type MockCustomizeStore struct {
	customizestore.StoreInterfaceForTest
}

// NewMockCustomizeStore create a customize store mock instance
func NewMockCustomizeStore() *MockCustomizeStore {
	cs := customizestore.NewCustomizeStoreManager(10*time.Minute, &types.OnlineConfig{Enable: true})
	cst, ok := cs.(customizestore.StoreInterfaceForTest)
	if !ok {
		klog.Fatalf("can't mock customize store for test")
	}
	return &MockCustomizeStore{StoreInterfaceForTest: cst}
}

// MockPromStore mock prometheus resource store
type MockPromStore struct{}

// mock function
func (prom *MockPromStore) GetPromDirectMetrics() (metricFamilies map[string]*dto.MetricFamily, err error) {
	panic("implement me")
}

// mock function
func (prom *MockPromStore) GetPromRangeMetrics(key string, start, end time.Time, count int) ([]float64, error) {
	panic("implement me")
}

// mock function
func (prom *MockPromStore) GetPromRecentMetrics(key string) (float64, error) {
	panic("implement me")
}
