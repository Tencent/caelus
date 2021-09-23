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

import "time"

// Detector defines detector methods
type Detector interface {
	// Name show detector name
	Name() string
	// Add add detect data
	Add(data TimedData)
	// AddAll add detect data array
	AddAll(vals []TimedData)
	// IsAnomaly checks if current values is anomaly
	IsAnomaly() (bool, error)
	// Metrics return current detector metrics
	Metrics() []string
	// SampleCount get current data count
	SampleCount() int
	// SampleDuration get current data time range
	SampleDuration() time.Duration
	// Reason get anomaly reason
	Reason() string
}
