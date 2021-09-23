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

package rulecheck

import (
	"reflect"
	"testing"
	"time"

	"github.com/tencent/caelus/pkg/caelus/detection"
	mock "github.com/tencent/caelus/pkg/caelus/detection/mock"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/util/times"
)

var (
	testGenerateRulesExpressionMetrics   = []string{"cpu", "memory"}
	testGenerateRulesExpressionCount     = 10
	testGenerateRulesExpressionDurations = times.Duration(5 * time.Second)
	testGenerateRulesEWMAMetrics         = "cpu"
	testGenerateRulesEWMACount           = 15
	testGenerateRulesSamples             = testGenerateRulesEWMACount
	testGenerateRulesDurations           = testGenerateRulesExpressionDurations
	testGenerateRulesDetectActionNames   = []string{types.DetectionExpression, types.DetectionEWMA, string(action.Evict),
		string(action.Schedule), types.DetectionEWMA, string(action.Schedule)}
	testGenerateRulesDetectActionRecoverNames = []string{types.DetectionExpression, string(action.Log)}
)

var testGenerateRulesConfig = &types.RuleCheckConfig{
	Name:            "testing",
	Metrics:         []string{"cpu", "memory"},
	CheckInterval:   times.Duration(1 * time.Second),
	HandleInterval:  times.Duration(2 * time.Second),
	RecoverInterval: times.Duration(5 * time.Second),
	Rules: []*types.DetectActionConfig{
		{
			Detects: []*types.DetectConfig{
				{
					Name: types.DetectionExpression,
					Args: &types.ExpressionArgs{
						Expression:      "cpu > 0.9 || memory > 0.8",
						WarningCount:    testGenerateRulesExpressionCount,
						WarningDuration: testGenerateRulesExpressionDurations,
					},
				},
				{
					Name: types.DetectionEWMA,
					Args: &types.EWMAArgs{
						Metric: testGenerateRulesEWMAMetrics,
						Nr:     testGenerateRulesEWMACount,
					},
				},
			},
			Actions: []*types.ActionConfig{
				{
					Name:    string(action.Evict),
					ArgsStr: []byte(""),
				},
				{
					Name:    string(action.Schedule),
					ArgsStr: []byte(""),
				},
			},
		},
		{
			Detects: []*types.DetectConfig{
				{
					Name: types.DetectionEWMA,
					Args: &types.EWMAArgs{
						Metric: testGenerateRulesEWMAMetrics,
						Nr:     testGenerateRulesEWMACount,
					},
				},
			},
			Actions: []*types.ActionConfig{
				{
					Name:    string(action.Schedule),
					ArgsStr: []byte(""),
				},
			},
		},
	},
	RecoverRules: []*types.DetectActionConfig{
		{
			Detects: []*types.DetectConfig{
				{
					Name: types.DetectionExpression,
					Args: &types.ExpressionArgs{
						Expression:      "cpu > 0.7",
						WarningCount:    testGenerateRulesExpressionCount,
						WarningDuration: testGenerateRulesExpressionDurations,
					},
				},
			},
			Actions: []*types.ActionConfig{
				{
					Name:    string(action.Log),
					ArgsStr: []byte(""),
				},
			},
		},
	},
}

// TestGenerateRules test parse rules function
func TestGenerateRules(t *testing.T) {
	rules, recoverRules, samples, duration := generateRules("checkerTest", testGenerateRulesConfig)
	if samples != testGenerateRulesSamples {
		t.Fatalf("testing generate health check rules, got wrong samples: %d, should be %d",
			samples, testGenerateRulesSamples)
	}
	if duration != testGenerateRulesDurations.TimeDuration() {
		t.Fatalf("testing generate health check rules, got wrong duration: %v, shoule be %v",
			duration, testGenerateRulesDurations)
	}

	if len(rules) != 2 || len(recoverRules) != 1 {
		t.Fatalf("testing generate health check rules, got wrong rules number(rules: %d, recoverRules: %d),"+
			"shoule be(2,1)", len(rules), len(recoverRules))
	}

	var detectActionNames []string
	for _, rule := range rules {
		for _, detect := range rule.detects {
			detectActionNames = append(detectActionNames, detect.Name())
		}
		for _, action := range rule.actions {
			detectActionNames = append(detectActionNames, string(action.ActionType()))
		}
	}
	if !reflect.DeepEqual(detectActionNames, testGenerateRulesDetectActionNames) {
		t.Fatalf("testing generate health check rules, got wrong rules names: %v, should %v",
			detectActionNames, testGenerateRulesDetectActionNames)
	}

	var detectActionRecoverNames []string
	for _, rule := range recoverRules {
		for _, detect := range rule.detects {
			detectActionRecoverNames = append(detectActionRecoverNames, detect.Name())
		}
		for _, action := range rule.actions {
			detectActionRecoverNames = append(detectActionRecoverNames, string(action.ActionType()))
		}
	}
	if !reflect.DeepEqual(detectActionRecoverNames, testGenerateRulesDetectActionRecoverNames) {
		t.Fatalf("testing generate health check rules, got wrong recover rules names: %v, should %v",
			detectActionRecoverNames, testGenerateRulesDetectActionRecoverNames)
	}
}

var (
	mockAnomaly                       = false
	recoverMockAnomaly                = false
	testRuleCheckMockDetection        = mock.NewMockDetection(&mockAnomaly, []string{"cpu"})
	testRuleCheckRecoverMockDetection = mock.NewMockDetection(&recoverMockAnomaly, []string{"cpu"})
	testRuleCheckHandleTime           = time.Now().Add(time.Duration(-6 * time.Second))
	testRuleCheckNoHandleTime         = time.Now().Add(time.Duration(-3 * time.Second))
	testRuleCheckRecoverTime          = time.Now().Add(time.Duration(-15 * time.Second))
	testRuleCheckNoRecoverime         = time.Now().Add(time.Duration(-9 * time.Second))

	testRuleCheckEntry = &ruleCheckEntry{
		name: "testing",
		rules: []*ruleEntry{
			{
				detects: []detection.Detector{
					testRuleCheckMockDetection,
				},
				actions: []action.Action{
					action.NewScheduleAction("testing"),
				},
			},
		},
		recoverRules: []*ruleEntry{
			{
				detects: []detection.Detector{
					testRuleCheckRecoverMockDetection,
				},
				actions: []action.Action{
					action.NewScheduleAction("testing"),
				},
			},
		},
		samples:         1,
		checkInterval:   time.Duration(1 * time.Second),
		handleInterval:  time.Duration(5 * time.Second),
		recoverInterval: time.Duration(10 * time.Second),
	}
)

var ruleCheckCases = []struct {
	describe              string
	mockAnomaly           bool
	recoverMockAnomaly    bool
	lastAbnormal          *time.Time
	lastHandle            *time.Time
	lastRecover           *time.Time
	expectDisableSchedule bool
	expectAnomaly         bool
	expectNoNeedHandle    bool
	expectNoNeedRecover   bool
}{
	{
		describe:              "check rule testing detecting normaly",
		mockAnomaly:           false,
		recoverMockAnomaly:    false,
		lastAbnormal:          nil,
		lastHandle:            nil,
		lastRecover:           nil,
		expectDisableSchedule: false,
		expectAnomaly:         false,
		expectNoNeedHandle:    false,
		expectNoNeedRecover:   false,
	},
	{
		describe:              "check rule testing detecting anomaly and handle action",
		mockAnomaly:           true,
		recoverMockAnomaly:    false,
		lastAbnormal:          nil,
		lastHandle:            &testRuleCheckHandleTime,
		lastRecover:           nil,
		expectDisableSchedule: true,
		expectAnomaly:         true,
		expectNoNeedHandle:    false,
		expectNoNeedRecover:   false,
	},
	{
		describe:              "check rule testing detecting anomaly and no need handle action",
		mockAnomaly:           true,
		recoverMockAnomaly:    false,
		lastAbnormal:          nil,
		lastHandle:            &testRuleCheckNoHandleTime,
		lastRecover:           nil,
		expectDisableSchedule: false,
		expectAnomaly:         true,
		expectNoNeedHandle:    true,
		expectNoNeedRecover:   false,
	},
	{
		describe:              "check rule testing detecting recover anomaly and handle action",
		mockAnomaly:           false,
		recoverMockAnomaly:    true,
		lastAbnormal:          nil,
		lastHandle:            nil,
		lastRecover:           &testRuleCheckRecoverTime,
		expectDisableSchedule: false,
		expectAnomaly:         false,
		expectNoNeedHandle:    false,
		expectNoNeedRecover:   false,
	},
	{
		describe:              "check rule testing detecting recover anomaly and no need handle action",
		mockAnomaly:           false,
		recoverMockAnomaly:    true,
		lastAbnormal:          nil,
		lastHandle:            nil,
		lastRecover:           &testRuleCheckNoRecoverime,
		expectDisableSchedule: false,
		expectAnomaly:         false,
		expectNoNeedHandle:    false,
		expectNoNeedRecover:   true,
	},
}

// TestCheckRule test checking rule
func TestCheckRule(t *testing.T) {
	metricsValuesFunc := func(metrics []string, samples int, duration time.Duration) []detection.TimedData {
		return []detection.TimedData{}
	}

	for _, tCase := range ruleCheckCases {
		t.Log(tCase.describe)

		mockAnomaly = tCase.mockAnomaly
		recoverMockAnomaly = tCase.recoverMockAnomaly
		testRuleCheckEntry.lastAbnormal = tCase.lastAbnormal
		if tCase.lastHandle != nil {
			testRuleCheckEntry.lastHandle = tCase.lastHandle
		}
		if tCase.lastRecover != nil {
			testRuleCheckEntry.lastRecover = tCase.lastRecover
		}
		acRet, anomaly, noNeedHandle, noNeedRecover := checkRule(testRuleCheckEntry, metricsValuesFunc, time.Now())
		if acRetDisableSchedule(acRet) != tCase.expectDisableSchedule || anomaly != tCase.expectAnomaly ||
			noNeedHandle != tCase.expectNoNeedHandle || noNeedRecover != tCase.expectNoNeedRecover {
			t.Fatalf("rule check testing fail, should do action, result: %v, anomaly: %v,"+
				"noNeedHandle: %v, noNeedRecover: %v", acRet, anomaly, noNeedHandle, noNeedRecover)
		}
	}
}

func acRetDisableSchedule(acRet *action.ActionResult) bool {
	for _, s := range acRet.UnscheduleMap {
		if s {
			return true
		}
	}

	return false
}
