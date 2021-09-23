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
	"strings"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type unionDetector struct {
	metrics   []string
	detectors []Detector

	reason []string
}

var _ Detector = (*unionDetector)(nil)

// NewUnionDetector union multiple detectors
func NewUnionDetector(detectors []Detector) Detector {
	metrics := sets.NewString()
	for _, d := range detectors {
		metrics.Insert(d.Metrics()...)
	}
	return &unionDetector{
		metrics:   metrics.List(),
		detectors: detectors,
	}
}

// Name show detector name
func (u *unionDetector) Name() string {
	return types.DetectionUnion
}

// Add add detect data
func (u *unionDetector) Add(data TimedData) {
	for _, d := range u.detectors {
		d.Add(data)
	}
}

// AddAll add detect data array
func (u *unionDetector) AddAll(vals []TimedData) {
	for _, d := range u.detectors {
		d.AddAll(vals)
	}
}

// IsAnomaly checks if current values is anomaly
func (u *unionDetector) IsAnomaly() (bool, error) {
	var reason []string
	for _, d := range u.detectors {
		ret, err := d.IsAnomaly()
		if err != nil {
			return false, err
		}
		if !ret {
			return false, nil
		}
		reason = append(reason, d.Reason())
	}
	u.reason = reason
	return true, nil
}

// Metrics return current detector metrics
func (u *unionDetector) Metrics() []string {
	return u.metrics
}

// Reason get anomaly reason
func (u *unionDetector) Reason() string {
	return strings.Join(u.reason, ";")
}

// SampleCount get current data count
func (u *unionDetector) SampleCount() int {
	var n int
	for _, d := range u.detectors {
		nn := d.SampleCount()
		if nn > n {
			n = nn
		}
	}

	return n
}

// SampleDuration get current data time range
func (u *unionDetector) SampleDuration() time.Duration {
	var duration time.Duration
	for _, d := range u.detectors {
		dd := d.SampleDuration()
		if dd > duration {
			duration = dd
		}
	}

	return duration
}
