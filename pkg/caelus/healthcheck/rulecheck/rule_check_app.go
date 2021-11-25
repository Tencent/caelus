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
	"strings"
	"time"

	"github.com/tencent/caelus/pkg/caelus/detection"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/customize"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"k8s.io/api/core/v1"
	"k8s.io/klog"
)

const healthCheckerApp = "AppHealthChecker"

type appHealthChecker struct {
	stStore             statestore.StateStore
	appDetectors        []detection.Detector
	conflictResources   []*action.ActionResource
	ruleCheckers        []*ruleCheckEntry
	adjustResourceIndex int
	checkInterval       time.Duration
	lastCheck           time.Time

	historySlo     []sloParam
	latestMetricTs time.Time
}

// sloParam show the further processing result for slo metric
type sloParam struct {
	min float64
	max float64
	ts  time.Time
}

func newAppHealthChecker(stStore statestore.StateStore, ruleCheckCfgs []*types.RuleCheckConfig) ruleChecker {
	appChecker := &appHealthChecker{
		stStore: stStore,
	}

	for _, rChecker := range ruleCheckCfgs {
		entries, recoverEntries, samples, duration := generateRules(healthCheckerApp, rChecker)
		name := rChecker.Name
		if name != "slo" {
			// metric name must be format, like osd:latency
			parts := strings.Split(name, ":")
			if len(parts) != 2 {
				klog.Fatalf("unknown app rule name %s", name)
			}
		}

		appChecker.ruleCheckers = append(appChecker.ruleCheckers,
			&ruleCheckEntry{
				name:            rChecker.Name,
				rules:           entries,
				recoverRules:    recoverEntries,
				samples:         samples,
				duration:        duration,
				checkInterval:   rChecker.CheckInterval.TimeDuration(),
				handleInterval:  rChecker.HandleInterval.TimeDuration(),
				recoverInterval: rChecker.RecoverInterval.TimeDuration(),
				lastCheck:       time.Now().Add(-rChecker.CheckInterval.TimeDuration()),
			})
	}

	return appChecker
}

// Name return current checker name
func (a *appHealthChecker) Name() string {
	return healthCheckerApp
}

// Check describe how to check app metrics rule, and output the result
func (a *appHealthChecker) Check() *action.ActionResult {
	acResult := &action.ActionResult{
		UnscheduleMap:   make(map[string]bool),
		AdjustResources: make(map[v1.ResourceName]action.ActionResource),
	}
	now := time.Now()
	cStates, err := a.stStore.ListCustomizeResourceRecentState()
	if err != nil {
		klog.Errorf("list customize resource recent state err: %v", err)
		return acResult
	}
	if len(cStates) == 0 {
		klog.V(5).Infof("list customize resource recent state, got nil")
		return acResult
	}

	// checking if the state has new metrics data
	needCheck := false
	for _, metric := range cStates {
		if metric.Ts.After(a.latestMetricTs) {
			needCheck = true
			a.latestMetricTs = metric.Ts
		}
	}
	if !needCheck {
		return acResult
	}

	for k, v := range cStates {
		metrics.OnlineJobsMetrics(k, v.Values)
	}

	// for slo metric, just choose the max and min value
	max, min := a.findMaxAndMinSLOSlack(cStates)
	a.historySlo = append(a.historySlo, sloParam{min: min, max: max, ts: now})
	if len(a.historySlo) > 100 {
		a.historySlo = a.historySlo[1:]
	}
	klog.V(2).Infof("slo slack info: min(%v), max(%v)", min, max)

	for _, rChecker := range a.ruleCheckers {
		if !rChecker.lastCheck.Add(rChecker.checkInterval).Before(now) {
			continue
		}
		// construct metric value function for rule check
		var f metricsValuesFunc
		if rChecker.name == "slo" {
			// special for slo metric
			f = a.sloMetricFunc
		} else {
			parts := strings.Split(rChecker.name, ":")
			cMetrics, err := a.stStore.GetCustomizeResourceRangeStats(parts[0],
				parts[1], now.Add(-30*time.Minute), now, 30)
			if err != nil {
				if err != util.ErrNotFound {
					klog.Warningf("failed get metric value of %s", rChecker.name)
				}
				continue
			}
			f = func(metrics []string, samples int, duration time.Duration) []detection.TimedData {
				var datas []detection.TimedData
				for _, m := range cMetrics {
					datas = append(datas, detection.TimedData{
						Ts:   m.Ts,
						Vals: m.Values,
					})
				}
				return datas
			}
		}

		// calling common check function
		rChecker.lastCheck = now
		acRet, anomaly, noNeedHandle, _ := checkRule(rChecker, f, now)
		acResult.Merge(acRet)
		if anomaly && !noNeedHandle {
			acResult.SyncEvent = true
			klog.V(2).Infof("finding resource %s conflicting, handle action: %s",
				rChecker.name, acResult)
		}
	}
	return acResult
}

// sloMetricFunc return fixed max and min metrics
func (a *appHealthChecker) sloMetricFunc(metrics []string, samples int, duration time.Duration) []detection.TimedData {
	var vals []detection.TimedData
	var hisLen = len(a.historySlo)
	if hisLen == 0 {
		klog.Warningf("empty slo metrics")
		return vals
	}

	// check sample numbers
	index := 0
	if samples > 0 && hisLen > samples {
		index = hisLen - samples
	}
	// check time range
	startTime := &time.Time{}
	if duration != 0 {
		startT := vals[len(vals)-1].Ts.Add(-duration)
		startTime = &startT
	}
	for i, slo := range a.historySlo[index:] {
		if startTime != nil {
			// check time duration, add extra one element in case the warning_duration not match
			if startTime.After(slo.ts) && (i+1) != hisLen && startTime.After(a.historySlo[i+1].ts) {
				continue
			}
		}
		data := detection.TimedData{Ts: slo.ts, Vals: map[string]float64{"slo_max": slo.max, "slo_min": slo.min}}
		vals = append(vals, data)
	}

	return vals
}

func (a *appHealthChecker) findMaxAndMinSLOSlack(metric map[string]*customizestore.Metric) (max float64, min float64) {
	max = 0.0 // when max <=0, it is meaningless and does not matter
	min = 1.0 // min cant be greater than 1.0
	for _, o := range metric {
		slo, _ := o.Values["slo"]
		value, _ := o.Values["value"]
		// FIXME: skip it for safety now.
		if value == 0 || slo == 0 {
			continue
		}
		slack := (slo - value) / (slo)
		if slack > max {
			max = slack
		}
		if slack < min {
			min = slack
		}
		if slack < 0 {
			metrics.SLONotMetCounterInc(fmt.Sprintf("%v", o.JobName))
		}
	}
	return max, min
}
