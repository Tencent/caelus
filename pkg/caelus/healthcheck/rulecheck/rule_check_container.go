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
	"github.com/tencent/caelus/pkg/caelus/healthcheck/rulecheck/correlation"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const healthCheckerContainer = "ContainerHealthChecker"

var (
	// fixed supported metrics
	cgroupSupportedMetrics = sets.NewString()
	// self defined supported metrics
	extraMetric = []string{"cpu_request"}
)

type containerHealthChecker struct {
	stStore     statestore.StateStore
	podInformer cache.SharedIndexInformer

	rulesFactory    []detectorFactory
	containerChecks map[cgroupstore.CgroupRef][]detection.Detector

	checkInterval time.Duration
	lastCheck     time.Time
}

func newContainerHealthChecker(stStore statestore.StateStore,
	podInformer cache.SharedIndexInformer, ruleCheckCfgs []*types.RuleCheckConfig) ruleChecker {
	checker := &containerHealthChecker{
		stStore:         stStore,
		podInformer:     podInformer,
		containerChecks: make(map[cgroupstore.CgroupRef][]detection.Detector),
	}

	for _, rCheck := range ruleCheckCfgs {
		// check if the metric is invalid
		for _, metric := range rCheck.Metrics {
			// initialize supported metrics, just once
			if cgroupSupportedMetrics.Len() == 0 {
				for _, k := range stStore.GetCgroupStoreSupportedTags() {
					cgroupSupportedMetrics.Insert(k)
				}
				cgroupSupportedMetrics.Insert(extraMetric...)
			}

			if !cgroupSupportedMetrics.Has(metric) {
				klog.Fatalf("invalid metric %s, supporting: %v", metric, cgroupSupportedMetrics)
			}
		}
		// now only parse detect rules, no recover rules
		for _, rule := range rCheck.Rules {
			ruleFactory, _, _, _, err := parseDetectorRule(healthCheckerContainer, rCheck.Name, rCheck.Metrics, rule)
			if err != nil {
				klog.Errorf("detect container rule err: %v", err)
				continue
			}
			checker.rulesFactory = append(checker.rulesFactory, ruleFactory...)
		}

		if !(checker.checkInterval.Seconds() != 0 && rCheck.CheckInterval.Seconds() >= checker.checkInterval.Seconds()) {
			checker.checkInterval = rCheck.CheckInterval.TimeDuration()
		}
	}
	checker.lastCheck = time.Now().Add(-checker.checkInterval)

	return checker
}

// Name return current checker name
func (c *containerHealthChecker) Name() string {
	return healthCheckerContainer
}

// The function pick suspicious pods, which may interference online jobs,
// when online jobs' metrics are detected abnormally
func (c *containerHealthChecker) Check() *action.ActionResult {
	ac := &action.ActionResult{
		UnscheduleMap:   make(map[string]bool),
		AdjustResources: make(map[v1.ResourceName]action.ActionResource),
	}

	// check periodically
	if c.checkInterval.Seconds() == 0 || !c.lastCheck.Add(c.checkInterval).Before(time.Now()) {
		return ac
	}
	c.lastCheck = time.Now()

	// get online jobs' resource stats, whose metrics are abnormal
	victimStats, existRefs := c.victimStats()
	if len(victimStats) == 0 {
		return ac
	}
	suspects := make(map[k8stypes.NamespacedName]struct{})
	for category, cstat := range victimStats {
		// choosing the most suspicious pods
		suspect := correlation.Analysis(category, cstat, c.stStore, c.podInformer)
		if suspect != nil {
			key := k8stypes.NamespacedName{Namespace: suspect.PodNamespace, Name: suspect.PodName}
			suspects[key] = struct{}{}
		}
	}
	klog.Errorf("find suspect pods: %v, will handle it", suspects)
	alarm.SendAlarm(fmt.Sprintf("find suspect pods: %v, will handle it\n", suspects))
	// delete useless container cache
	for ref := range c.containerChecks {
		if _, exist := existRefs[ref]; !exist {
			delete(c.containerChecks, ref)
		}
	}
	// add suspicious pods to action result
	for k := range suspects {
		ac.EvictPods = append(ac.EvictPods, k)
	}

	return ac
}

// victimStats choose online jobs' resource state, whose metrics are abnormal
func (c *containerHealthChecker) victimStats() (
	victimStats map[string]map[cgroupstore.CgroupRef]*cgroupstore.CgroupStats,
	existRefs map[cgroupstore.CgroupRef]struct{}) {
	victimStats = make(map[string]map[cgroupstore.CgroupRef]*cgroupstore.CgroupStats)
	existRefs = make(map[cgroupstore.CgroupRef]struct{})

	// get all online jobs
	cstats, err := c.stStore.ListCgroupResourceRecentState(false, sets.NewString(appclass.AppClassOnline))
	if err != nil {
		klog.Errorf("failed list all container stats: %v", err)
		return
	}

	for _, stat := range cstats {
		if stat.Ref.ContainerName == "" || stat.Ref.PodNamespace == "kube-system" {
			continue
		}

		// check if online jobs' metrics are abnormal
		existRefs[*stat.Ref] = struct{}{}
		if _, exist := c.containerChecks[*stat.Ref]; !exist {
			var detectors []detection.Detector
			for _, factory := range c.rulesFactory {
				d := factory()
				detectors = append(detectors, d)
			}
			c.containerChecks[*stat.Ref] = detectors
		}
		detectors := c.containerChecks[*stat.Ref]
		for _, detector := range detectors {
			// add recent metrics data to detector
			metrics := detector.Metrics()
			params := make(map[string]float64)
			for _, metric := range metrics {
				params[metric] = c.getMetricValue(metric, stat)
			}
			detector.Add(detection.TimedData{Ts: stat.Timestamp, Vals: params})
			// check metrics data based on detector
			if isA, err := detector.IsAnomaly(); err == nil && isA {
				// save to victim stats if metrics are abnormal
				metrics := detector.Metrics()
				for _, metric := range metrics {
					category := metricCategory(metric)
					if category != "" {
						st := victimStats[category]
						if st == nil {
							st = make(map[cgroupstore.CgroupRef]*cgroupstore.CgroupStats)
							victimStats[category] = st
						}
						st[*stat.Ref] = stat
					}
				}

				msg := fmt.Sprintf("%s/%s/%s has abnormal on metrics: %v",
					stat.Ref.PodNamespace, stat.Ref.PodName, stat.Ref.ContainerName, params)
				klog.Error(msg)
				alarm.SendAlarm(msg)
			}
		}
	}

	return victimStats, existRefs
}

// metrics are classified by resource
func metricCategory(metric string) string {
	switch metric {
	case "mem_direct_reclaim", "mem_direct_compact", "memory_working_set_activate":
		return "memory"
	case "read_latency", "write_latency", "read_count", "write_count":
		return "diskio"
	default:
		return ""
	}
}

func (c *containerHealthChecker) getMetricValue(metric string, stat *cgroupstore.CgroupStats) float64 {
	switch metric {
	case "cpu_request":
		return c.getCpuRequest(stat.Ref.ContainerName, stat.Ref.PodName, stat.Ref.PodNamespace)
	default:
		// get fixed supported metric name from the struct data
		v, err := stat.GetValue(metric)
		if err != nil {
			klog.Errorf("get cgroup metric(%s) value err: %v", metric, err)
			v = 0
		}
		return v
	}
}

// getCpuRequest get container's cpu request value
func (c *containerHealthChecker) getCpuRequest(containerName, podName, namespace string) float64 {
	if podName == "" || namespace == "" {
		return 100
	}
	obj, exist, err := c.podInformer.GetStore().GetByKey(fmt.Sprintf("%s/%s", namespace, podName))
	if err != nil {
		klog.Errorf("failed get pod(%s/%s): %v", namespace, podName, err)
		return 100
	}
	if !exist {
		klog.Errorf("pod not found: %s/%s", namespace, podName)
		return 100
	}
	pod := obj.(*v1.Pod)
	for _, c := range pod.Spec.Containers {
		if c.Name == containerName {
			cpu := c.Resources.Requests.Cpu()
			if cpu == nil {
				return 100
			}
			cpuV := cpu.ScaledValue(resource.Milli)
			return float64(cpuV) / 1000
		}
	}
	return 100
}
