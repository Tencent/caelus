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

package detection

import (
	"time"

	"github.com/tencent/caelus/pkg/caelus/detection"
)

// MockDetection detection mock type
type MockDetection struct {
	anomaly         *bool
	sampleCount     int
	warningDuration time.Duration
	metrics         []string
	vals            []detection.TimedData
}

// NewMockDetection new detection mock manager
func NewMockDetection(anomaly *bool, metrics []string) detection.Detector {
	return &MockDetection{anomaly: anomaly, metrics: metrics}
}

// Name show detector name
func (m *MockDetection) Name() string {
	return "detectionTesting"
}

// Add add detect data
func (m *MockDetection) Add(data detection.TimedData) {
	m.vals = append(m.vals, data)
}

// AddAll add detect data array
func (m *MockDetection) AddAll(vals []detection.TimedData) {
	m.vals = vals
}

// IsAnomaly checks if current values is anomaly
func (m *MockDetection) IsAnomaly() (bool, error) {
	return *m.anomaly, nil
}

// Metrics return current detector metrics
func (m *MockDetection) Metrics() []string {
	return m.metrics
}

// SampleCount get current data count
func (m *MockDetection) SampleCount() int {
	return m.sampleCount
}

// SampleDuration get current data time range
func (m *MockDetection) SampleDuration() time.Duration {
	return m.warningDuration
}

// Reason get anomaly reason
func (m *MockDetection) Reason() string {
	return "mock testing"
}
