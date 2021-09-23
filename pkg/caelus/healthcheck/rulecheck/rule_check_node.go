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
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/tencent/caelus/pkg/caelus/detection"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/node"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	healthCheckerNode = "NodeHealthChecker"
	nodeMemory        = "memory"
)

var (
	nodeSupportedMetrics = sets.NewString()
	zeroTime             time.Time
)

type nodeHealthChecker struct {
	stStore           statestore.StateStore
	ruleCheckers      []*ruleCheckEntry
	predictedReserved v1.ResourceList

	// lock to protect top command
	cas int32
}

func newNodeHealthChecker(stStore statestore.StateStore, reserved *types.Resource,
	ruleChecks []*types.RuleCheckConfig) ruleChecker {
	ndChecker := &nodeHealthChecker{
		stStore: stStore,
		predictedReserved: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(int64(*reserved.CpuMilli), resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(int64(*reserved.MemMB)*types.MemUnit, resource.DecimalSI),
		},
		cas: 0,
	}

	for _, rChecker := range ruleChecks {
		// check if metrics are invalid
		for _, metric := range rChecker.Metrics {
			// drop device name, just keep metric name
			_, _, originalMetric := types.GetDeviceNameFromMetric(metric)
			// initialize supported metrics just once
			if nodeSupportedMetrics.Len() == 0 {
				for _, k := range stStore.GetNodeStoreSupportedTags() {
					nodeSupportedMetrics.Insert(k)
				}
			}
			if !nodeSupportedMetrics.Has(originalMetric) {
				klog.Fatalf("invalid metric %s, supporting: %v", metric, nodeSupportedMetrics)
			}
		}

		entries, recoverEntries, samples, duration := generateRules(healthCheckerNode, rChecker)
		for _, ruleEntry := range entries {
			ndChecker.generateSelfDefineHandler(ruleEntry.actions)
		}

		ndChecker.ruleCheckers = append(ndChecker.ruleCheckers,
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

	return ndChecker
}

// Name returns current checker name
func (n *nodeHealthChecker) Name() string {
	return healthCheckerNode
}

// Check describe how to check node metrics rule, and output the result
func (n *nodeHealthChecker) Check() *action.ActionResult {
	acResult := &action.ActionResult{
		UnscheduleMap:   make(map[string]bool),
		AdjustResources: make(map[v1.ResourceName]action.ActionResource),
	}

	now := time.Now()
	anomalyOccur := false
	for _, rChecker := range n.ruleCheckers {
		if !rChecker.lastCheck.Add(rChecker.checkInterval).Before(now) {
			continue
		}
		rChecker.lastCheck = now

		start := zeroTime
		if rChecker.duration.Seconds() != 0 {
			// add buffer, in case the strict time duration checking will be always not match
			buffer := rChecker.duration
			if rChecker.duration.Seconds() > 60 {
				buffer = time.Duration(1 * time.Minute)
			}
			start = now.Add(-rChecker.duration).Add(-buffer)
		}
		sts, err := n.stStore.GetNodeResourceRangeStats(start, zeroTime, rChecker.samples)
		if err != nil {
			klog.Errorf("get node resource range stats(start: %v) err: %v", start, err)
			continue
		}

		// construct metric function
		metricsValuesFunc := func(metrics []string, samples int, duration time.Duration) []detection.TimedData {
			return n.getNodeMetricsValues(metrics, sts, samples, duration)
		}

		// calling common rule check function
		acRet, anomaly, noNeedHandle, _ := checkRule(rChecker, metricsValuesFunc, now)
		anomalyOccur = anomalyOccur || anomaly
		acResult.Merge(acRet)
		if anomaly && !noNeedHandle {
			acResult.SyncEvent = true
			klog.V(2).Infof("finding resource %s conflicting, handle action: %+v",
				rChecker.name, acResult)
		}
	}
	if anomalyOccur {
		go n.logCurrentNodeState()
	}
	return acResult
}

func (n *nodeHealthChecker) logCurrentNodeState() {
	if !atomic.CompareAndSwapInt32(&n.cas, 0, 1) {
		return
	}
	cmd := exec.Command("top", "-b", "-d", "1", "-n", "2")
	output, _ := cmd.CombinedOutput()
	klog.Infof("current top state:")
	klog.Infof("%s", output)
	atomic.StoreInt32(&n.cas, 0)
}

// special initialize for different resource or action type, not for recover rules
func (n *nodeHealthChecker) generateSelfDefineHandler(actions []action.Action) {
	for _, ac := range actions {
		switch ac.ActionType() {
		case action.Adjust:
			var selfHandler action.AdjustSelfDefineFunc
			resName := ac.(*action.AdjustResourceAction).ResourceName()
			switch resName {
			case nodeMemory:
				selfHandler = n.getMemConflictedQuantity
			default:
			}

			if selfHandler != nil {
				ac.(*action.AdjustResourceAction).InitSelfHandler(resName, selfHandler)
			}
		default:
		}
	}
	return
}

// getMemConflictedQuantity, just for getting conflicting quantity for memory resource
func (n *nodeHealthChecker) getMemConflictedQuantity(conflicting bool) *action.ActionResource {
	var err error
	if !conflicting {
		// just handle in conflicting state
		return nil
	}

	// real used resource
	nodeSt, err := n.stStore.GetNodeResourceRecentState()
	if err != nil {
		klog.Errorf("get node state err: %v", err)
		return nil
	}

	overUsed := n.predictedReserved.Memory().Value() - int64(nodeSt.Memory.Available)
	klog.V(3).Infof("current memory usage in conflicting state, reserved:%d available:%d, overused:%d",
		n.predictedReserved.Memory().Value(), int64(nodeSt.Memory.Available), overUsed)
	if overUsed <= 0 {
		klog.Warningf("memory conflicting, while not reach reserved quantity, total: %f, rss: %f, reserved: %d",
			nodeSt.Memory.Total, nodeSt.Memory.UsageRss, n.predictedReserved.Memory().Value())
		return nil
	}

	return &action.ActionResource{
		Name:        v1.ResourceMemory,
		Conflicting: conflicting,
		ConflictQuantity: map[action.ActionFormula]resource.Quantity{
			// negative value
			action.FormulaTotal: *resource.NewQuantity(-overUsed, resource.DecimalSI),
		},
	}
}

// getNodeMetricsValues choose metrics value limited by sample number and time duration
func (n *nodeHealthChecker) getNodeMetricsValues(metrics []string, sts []*nodestore.NodeResourceState,
	samples int, duration time.Duration) []detection.TimedData {
	var vals []detection.TimedData
	if len(sts) == 0 {
		klog.Warningf("empty node resource stats")
		return vals
	}

	i := 0
	if samples > 0 && len(sts) > samples {
		// just keep sample number metrics
		i = len(sts) - samples
	}
	startTime := &time.Time{}
	if duration != 0 {
		startT := sts[len(sts)-1].Timestamp.Add(-duration)
		startTime = &startT
	}

	var st, stNext *nodestore.NodeResourceState
	for {
		if i == len(sts) {
			break
		}
		st = sts[i]
		i = i + 1

		stNext = nil
		if i != len(sts) {
			stNext = sts[i]
		}

		// check time duration, add extra one element in case the warning_duration not match
		if startTime != nil {
			if startTime.After(st.Timestamp) && stNext != nil && startTime.After(stNext.Timestamp) {
				continue
			}
		}

		params := make(map[string]float64)
		for _, metric := range metrics {
			// metric may need device name, such as disk io for different block devices
			device, devMetric, originalMetric := types.GetDeviceNameFromMetric(metric)
			m, err := st.GetValue(originalMetric, device)
			if err != nil {
				klog.Errorf("get metric(%s/%s) value err, assigning zero value: %v", originalMetric, device, err)
				m = 0
			}
			params[devMetric] = m
		}
		vals = append(vals, detection.TimedData{Ts: st.Timestamp, Vals: params})
	}

	return vals
}
