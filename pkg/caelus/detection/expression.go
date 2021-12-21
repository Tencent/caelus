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
	"github.com/Knetic/govaluate"
	"github.com/tencent/caelus/pkg/caelus/types"
	"time"
)

// ExpressionDetector using expressions to detect anomaly metrics
type ExpressionDetector struct {
	metrics       []string
	exp           *govaluate.EvaluableExpression
	params        map[string]interface{}
	recentAddTime time.Time

	// if both count and duration is set, return abnormal result when either is matched
	warningCount         int // 连续多次
	falseCount           int
	warningDuration      time.Duration // 持续多久
	firstContinueWarning time.Time

	renew bool // 检测到异常后是否重置

	reason string
}

var _ Detector = (*ExpressionDetector)(nil)

// NewExpressionDetector new an expression detector
func NewExpressionDetector(metrics []string, exp *govaluate.EvaluableExpression,
	options ...ExpressionOption) *ExpressionDetector {
	d := &ExpressionDetector{
		metrics: metrics,
		exp:     exp,
		params:  make(map[string]interface{}),
	}
	for _, opt := range options {
		opt(d)
	}
	if d.warningDuration == 0 && d.warningCount == 0 {
		d.warningCount = 1
	}
	return d
}

// Name show detector name
func (e *ExpressionDetector) Name() string {
	return types.DetectionExpression
}

// Add add detect data
func (e *ExpressionDetector) Add(data TimedData) {
	e.add(data)
}

func (e *ExpressionDetector) add(data TimedData) {
	if !data.Ts.After(e.recentAddTime) {
		return
	}
	e.recentAddTime = data.Ts
	for k, v := range data.Vals {
		e.params[k] = v
	}
	e.detect()
}

func (e *ExpressionDetector) detect() {
	ret, err := e.exp.Evaluate(e.params)
	if err != nil {
		return
	}
	if r, ok := ret.(bool); ok {
		if r {
			if e.warningCount > 0 {
				e.falseCount++
			}
			if e.warningDuration != 0 {
				if e.firstContinueWarning == nilTime {
					e.firstContinueWarning = e.recentAddTime
				}
			}
		} else {
			e.falseCount = 0
			e.firstContinueWarning = nilTime
		}
	}
}

// AddAll add detect data array
func (e *ExpressionDetector) AddAll(vals []TimedData) {
	e.recentAddTime = nilTime
	e.falseCount = 0
	e.firstContinueWarning = nilTime
	for _, val := range vals {
		e.add(val)
	}
}

// IsAnomaly checks if current values is anomaly
func (e *ExpressionDetector) IsAnomaly() (bool, error) {
	var ret bool

	if e.warningCount > 0 && e.firstContinueWarning != nilTime {
		warningCountRes := e.falseCount >= e.warningCount
		warningDurationRes := e.recentAddTime.Sub(e.firstContinueWarning) >= e.warningDuration
		ret = warningCountRes || warningDurationRes
	} else if e.warningCount > 0 {
		ret = e.falseCount >= e.warningCount
	} else if e.firstContinueWarning != nilTime {
		ret = e.recentAddTime.Sub(e.firstContinueWarning) >= e.warningDuration
	}

	if e.renew {
		e.falseCount = 0
		e.firstContinueWarning = nilTime
	}
	if ret {
		params := make(map[string]interface{})
		for _, v := range e.exp.Vars() {
			val := e.params[v]
			params[v] = val
		}
		e.reason = fmt.Sprintf("expression(%s) abnormal", e.exp.String())
	}
	return ret, nil
}

// Metrics return current detector metrics
func (e *ExpressionDetector) Metrics() []string {
	return e.metrics
}

// SampleCount get current data count
func (e *ExpressionDetector) SampleCount() int {
	return e.warningCount
}

// SampleDuration get current data time range
func (e *ExpressionDetector) SampleDuration() time.Duration {
	return e.warningDuration
}

// Reason get anomaly reason
func (e *ExpressionDetector) Reason() string {
	return e.reason
}

type ExpressionOption func(*ExpressionDetector) error

// ExpressionWarningArgs is used to set some expression args
func ExpressionWarningArgs(args *types.ExpressionArgs) ExpressionOption {
	return func(e *ExpressionDetector) error {
		e.warningCount = args.WarningCount
		e.warningDuration = args.WarningDuration.TimeDuration()
		e.firstContinueWarning = nilTime
		return nil
	}
}
