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
	"fmt"
	"math"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/tencent/caelus/pkg/caelus/detection/ring"
	"github.com/tencent/caelus/pkg/caelus/types"
)

// MinData is the minimum length of data for AnomalyDetector to work
const MinData = 11

// EwmaDetector using ewma alg to detect anomaly values
type EwmaDetector struct {
	data          ring.Ring
	n             int
	ma            ewma.MovingAverage
	recentAddTime time.Time
	addCount      int
	metric        string
}

var _ Detector = (*EwmaDetector)(nil)

// NewEwmaDetector returns a AnomalyDetector which keeps n data.
// Note: please initialize the detector with at lease 10 data using Add() method.
// It is required by the EWMA library to be "ready" for producing value.
// AnomalyDetector is not thread safe
func NewEwmaDetector(metric string, n int) *EwmaDetector {
	return &EwmaDetector{
		ma:            ewma.NewMovingAverage(float64(n)),
		data:          ring.NewRing(n),
		recentAddTime: nilTime,
		metric:        metric,
		n:             n,
	}
}

// Name show detector name
func (e *EwmaDetector) Name() string {
	return types.DetectionEWMA
}

// Add add detect data
func (e *EwmaDetector) Add(data TimedData) {
	e.add(data)
}

func (e *EwmaDetector) add(data TimedData) {
	if !data.Ts.After(e.recentAddTime) {
		return
	}
	e.recentAddTime = data.Ts
	if e.addCount < MinData {
		e.addCount++
	}
	e.ma.Add(data.Vals[e.metric])
	e.data.Add(data.Vals[e.metric])
}

// AddAll add detect data array
func (e *EwmaDetector) AddAll(vals []TimedData) {
	e.ma = ewma.NewMovingAverage(float64(e.n))
	e.data = ring.NewRing(e.n)
	e.recentAddTime = nilTime
	e.addCount = 0
	for _, val := range vals {
		e.add(val)
	}
}

// IsAnomaly checks if current values is anomaly
func (e *EwmaDetector) IsAnomaly() (bool, error) {
	if e.addCount < MinData {
		return false, fmt.Errorf("too few samples(%d), at least %d", e.addCount, MinData)
	}
	return math.Abs(e.data.Peek()-e.Mean()) > 3*e.StdDev(), nil
}

// Mean returns the mean value
func (e *EwmaDetector) Mean() float64 {
	return e.ma.Value()
}

// StdDev returns the standard deviation
func (e *EwmaDetector) StdDev() float64 {
	// The algorithm reference: https://zh.wikipedia.org/wiki/%E6%A8%99%E6%BA%96%E5%B7%AE
	var sum float64
	mean := e.Mean()
	for _, v := range e.data.Values() {
		d := math.Abs(v - mean)
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(e.data.Values())))
}

// Metrics return current detector metrics
func (e *EwmaDetector) Metrics() []string {
	return []string{e.metric}
}

// SampleCount get current data count
func (e *EwmaDetector) SampleCount() int {
	return e.n
}

// SampleDuration get current data time range
func (e *EwmaDetector) SampleDuration() time.Duration {
	return time.Duration(0)
}

// Reason get anomaly reason
func (e *EwmaDetector) Reason() string {
	return fmt.Sprintf("ewma abnormal with metric %s", e.metric)
}
