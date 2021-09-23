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
	"fmt"
	"time"

	"github.com/tencent/caelus/pkg/caelus/alarm"
	"github.com/tencent/caelus/pkg/caelus/detection"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	prom "github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/types"

	"github.com/Knetic/govaluate"
	"k8s.io/api/core/v1"
	"k8s.io/klog"
)

// detectorFactory is the wrap function, generating different detectors with same parameter
type detectorFactory func() detection.Detector

// metricsValuesFunc output time series data based on metrics name,
// the metrics numbers are limited by sample number and time range
type metricsValuesFunc func(metrics []string, samples int, duration time.Duration) []detection.TimedData

// ruleEntry group different detectors and actions
type ruleEntry struct {
	detects []detection.Detector
	actions []action.Action
}

type ruleCheckEntry struct {
	name string
	// rules used to check when interference is coming
	rules []*ruleEntry
	// recoverRules used to check when interference is gone
	recoverRules []*ruleEntry
	// max metrics sample numbers for this rule
	samples int
	// max metrics duration for this rule
	duration        time.Duration
	checkInterval   time.Duration
	handleInterval  time.Duration
	recoverInterval time.Duration
	lastCheck       time.Time
	lastHandle      *time.Time
	lastRecover     *time.Time
	lastAbnormal    *time.Time
}

// generateRules read from config file, and generate related rule entry struct data
func generateRules(checker string, rc *types.RuleCheckConfig) (
	rules, recoverRules []*ruleEntry, samples int, duration time.Duration) {
	// rules for interference
	for _, rule := range rc.Rules {
		entry, curSamples, curDuration, err := generateDetectActionEntry(checker, rc.Name, rc.Metrics, rule)
		if err != nil {
			klog.Errorf("parse %s detect rule err: %v", checker, err)
			continue
		}

		rules = append(rules, entry)
		if curSamples > samples {
			samples = curSamples
		}
		if curDuration > duration {
			duration = curDuration
		}
	}

	// rules checking when interference is gone
	for _, rule := range rc.RecoverRules {
		entry, curSamples, curDuration, err := generateDetectActionEntry(checker, rc.Name, rc.Metrics, rule)
		if err != nil {
			klog.Errorf("parse %s recover rule err: %v", checker, err)
			continue
		}

		recoverRules = append(recoverRules, entry)
		if curSamples > samples {
			samples = curSamples
		}
		if curDuration > duration {
			duration = curDuration
		}
	}
	if len(recoverRules) > 0 {
		klog.V(2).Infof("%s rule check %s has recover rules", checker, rc.Name)
	}

	if samples == 0 {
		// -1 means no number limit
		samples = -1
	}

	return rules, recoverRules, samples, duration
}

func generateDetectActionEntry(checker, ruleName string, metrics []string, ruleCfg *types.DetectActionConfig) (
	rule *ruleEntry, samples int, duration time.Duration, err error) {
	detectFactories, actions, curSamples, curDuration, curErr := parseDetectorRule(checker, ruleName, metrics, ruleCfg)
	if curErr != nil {
		err = fmt.Errorf("parse %s detect rule(%s) err: %v", checker, ruleName, err)
		return
	}

	var detects []detection.Detector
	for _, f := range detectFactories {
		detects = append(detects, f())
	}
	rule = &ruleEntry{
		detects: detects,
		actions: actions,
	}
	samples = curSamples
	duration = curDuration
	return rule, samples, duration, err
}

// parseDetectorRule parse signal rule and generate detectors and actions
func parseDetectorRule(checker, ruleName string, ruleMetrics []string, detectAc *types.DetectActionConfig) (
	factories []detectorFactory, actions []action.Action, samples int, duration time.Duration, err error) {
	if detectAc == nil {
		return factories, actions, samples, duration, fmt.Errorf("nil rule action")
	}

	// parse detector
	for _, detector := range detectAc.Detects {
		factory, curSamples, curDuration, err := generateDetectorFactory(ruleMetrics, detector)
		if err != nil {
			klog.Fatalf("%s generate detector err: %v", checker, err)
		}
		// choose max metrics samples number
		if curSamples > samples {
			samples = curSamples
		}
		// choose max metrics duration
		if curDuration > duration {
			duration = curDuration
		}
		factories = append(factories, factory)
	}

	// parse action
	for _, actor := range detectAc.Actions {
		ac, err := generateAction(actor, ruleName)
		if err != nil {
			klog.Fatalf("%s generate action err: %v", checker, err)
		}
		actions = append(actions, ac)
	}

	return factories, actions, samples, duration, nil
}

// generateDetectorFactory parse detector from config file
func generateDetectorFactory(ruleMetrics []string, detector *types.DetectConfig) (
	factory detectorFactory, samples int, duration time.Duration, err error) {
	if len(detector.Name) == 0 {
		err = fmt.Errorf("find nil detect name")
		return
	}

	// detectors with isolated method, we need to add extra codes when new detector is appended
	switch detector.Name {
	case types.DetectionExpression:
		var exp *govaluate.EvaluableExpression
		args := detector.Args.(*types.ExpressionArgs)
		exp, err = govaluate.NewEvaluableExpression(args.Expression)
		if err != nil {
			err = fmt.Errorf("failed parse expression(%s): %v", args.Expression, err)
			return
		}
		samples = args.WarningCount
		duration = args.WarningDuration.TimeDuration()
		factory = func() detection.Detector {
			return detection.NewExpressionDetector(ruleMetrics, exp,
				detection.ExpressionWarningArgs(args))
		}
	case types.DetectionEWMA:
		args := detector.Args.(*types.EWMAArgs)
		if args.Nr < detection.MinData {
			args.Nr = detection.MinData
		}
		samples = args.Nr
		metric := args.Metric
		if len(metric) == 0 {
			metric = ruleMetrics[0]
		}
		factory = func() detection.Detector {
			return detection.NewEwmaDetector(metric, samples)
		}
	default:
		err = fmt.Errorf("unknown detect %s, skip", detector.Name)
	}

	return factory, samples, duration, err
}

// generateAction parse action from config file
func generateAction(actor *types.ActionConfig, ruleName string) (ac action.Action, err error) {
	// actions with isolated method, we need to add extra codes when new action is appended
	switch actor.Name {
	case "", string(action.Log):
		ac = action.NewLogAction()
	case string(action.Schedule):
		ac = action.NewScheduleAction(ruleName)
	case string(action.Adjust):
		ac = action.NewAdjustResourceAction(v1.ResourceName(ruleName), actor.ArgsStr)
	case string(action.Evict):
		ac = action.NewEvictAction()
	default:
		err = fmt.Errorf("invalid action name: %s", actor.Name)
	}

	return ac, err
}

// checkRule is the main rule checking function.
// The function will check interference rules firstly, and handle recover rules when interference is gone. All checking
// and handling will be done when timestamp is available.
func checkRule(checker *ruleCheckEntry, metricsValuesFunc metricsValuesFunc, now time.Time) (
	acRet *action.ActionResult, anomaly, noNeedHandle, noNeedRecover bool) {
	acRet = &action.ActionResult{
		UnscheduleMap:   make(map[string]bool),
		AdjustResources: make(map[v1.ResourceName]action.ActionResource),
	}

	// check interference rules
	var actions []action.Action
	var actionsAnomaly []bool
	for i, rule := range checker.rules {
		curAnomaly, err := detect(rule.detects, metricsValuesFunc, true)
		if err != nil {
			klog.Errorf("check node metrics(%s[%d]) err: %v", checker.name, i, err)
			continue
		}
		if curAnomaly {
			// anomaly is true if one of detectors checks abnormally
			anomaly = true
		}

		// save checking result for different detectors, to handle right actions
		var curActionsAnomaly []bool
		for range rule.actions {
			curActionsAnomaly = append(curActionsAnomaly, curAnomaly)
		}
		actions = append(actions, rule.actions...)
		actionsAnomaly = append(actionsAnomaly, curActionsAnomaly...)
	}

	reason := fmt.Sprintf("%s detect NORMAL", checker.name)
	if anomaly {
		// check if the timestamp is available
		reason = fmt.Sprintf("%s detect ABNORMAL", checker.name)
		checker.lastAbnormal = &now
		if checker.lastHandle != nil && !checker.lastHandle.Add(checker.handleInterval).Before(now) {
			klog.V(4).Infof("metrics %s is ABNORMAL and has been handled at %s, "+
				"it won't be tuned again until %s", checker.name, checker.lastHandle.Format(time.Stamp),
				checker.lastHandle.Add(checker.handleInterval).Format(time.Stamp))
			return acRet, true, true, false
		}
		checker.lastHandle = &now

		prom.InterferenceCounterInc(checker.name)
	} else {
		// interference is gone, calling recover rules
		if checker.lastAbnormal != nil &&
			(checker.lastRecover == nil || checker.lastRecover.Before(*checker.lastAbnormal)) {
			checker.lastRecover = checker.lastAbnormal
		}
		if checker.lastRecover != nil && !checker.lastRecover.Add(checker.recoverInterval).Before(now) {
			klog.V(4).Infof("metrics %s is NORMAL and has just recovered at %s, "+
				"it won't be tuned again until %s", checker.name, checker.lastRecover.Format(time.Stamp),
				checker.lastRecover.Add(checker.recoverInterval).Format(time.Stamp))
			return acRet, false, false, true
		}
		checker.lastRecover = &now

		recoverAnomaly, recoverActions, recoverActionsAnomaly := handleRecover(checker, metricsValuesFunc)
		if recoverAnomaly {
			reason = fmt.Sprintf("%s recover detect ABNORMAL", checker.name)
			actions = recoverActions
			actionsAnomaly = recoverActionsAnomaly
		}
	}

	mergeAction(acRet, actions, actionsAnomaly, reason, checker.name)
	return acRet, anomaly, noNeedHandle, noNeedRecover
}

func handleRecover(checker *ruleCheckEntry, metricsValuesFunc metricsValuesFunc) (
	recoverAnomaly bool, recoverActions []action.Action, recoverActionsAnomaly []bool) {
	if len(checker.recoverRules) == 0 {
		return
	}
	for i, recoverRule := range checker.recoverRules {
		rAnomaly, err := detect(recoverRule.detects, metricsValuesFunc, false)
		if err != nil {
			klog.Errorf("recover check node metrics(%s[%d]) err: %v", checker.name, i, err)
			continue
		}
		if rAnomaly {
			recoverAnomaly = true
			recoverActions = append(recoverActions, recoverRule.actions...)
			for range recoverRule.actions {
				// recover should pass Non-confilcting when handle action, which to recover something
				recoverActionsAnomaly = append(recoverActionsAnomaly, false)
			}
		}
	}
	if recoverAnomaly {
		klog.V(3).Infof("metrics(%s) recover detection matched, doing recover actions",
			checker.name)
	}

	return recoverAnomaly, recoverActions, recoverActionsAnomaly
}

// mergeAction merge different action results
func mergeAction(acRet *action.ActionResult, actions []action.Action, actionsAnomaly []bool, reason, checker string) {
	for i, ac := range actions {
		var data interface{}
		switch ac.ActionType() {
		case action.Adjust:
			fallthrough
		case action.Schedule:
			fallthrough
		case action.Log:
			data = reason
		}
		aRet, err := ac.DoAction(actionsAnomaly[i], data)
		if err != nil {
			klog.Errorf("%s action(%s) handle err: %v", checker, ac.ActionType(), err)
			continue
		}
		klog.V(4).Infof("%s action type %s with result %+v", checker, ac.ActionType(), aRet)
		acRet.Merge(aRet)
	}
}

func detect(detectors []detection.Detector, metricsValuesFunc metricsValuesFunc, warning bool) (bool, error) {
	for _, d := range detectors {
		// add metrics values to detector
		metrics := d.Metrics()
		vals := metricsValuesFunc(metrics, d.SampleCount(), d.SampleDuration())
		if klog.V(4) {
			klog.Infof("metrics: %s", metrics)
			klog.Infof("values : %+v", vals)
		}
		d.AddAll(vals)
		anomaly, err := d.IsAnomaly()
		if err != nil {
			klog.Errorf("one of node detect(metrics:%s) err: %v", metrics, err)
			return false, err
		}
		if warning && anomaly {
			alarm.SendAlarm(d.Reason())
		}
		if anomaly {
			return true, nil
		}
	}

	return false, nil
}
