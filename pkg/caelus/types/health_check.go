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

package types

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/tencent/caelus/pkg/caelus/util/machine"
	"github.com/tencent/caelus/pkg/util/times"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	defaultRuleInterval = times.Duration(1 * time.Minute)
	defaultIoUtilMax    = 10

	defaultExpressionWarningCount = 1
	ExpresstionAutoDetect         = "auto"

	DetectionExpression = "expression"
	DetectionEWMA       = "ewma"
	DetectionUnion      = "union"

	ruleNameCpu     = "cpu"
	ruleNameMemmory = "memory"
	ruleNameDiskIO  = "diskio"
	ruleNameNetIO   = "netio"

	genericDev         = "$dev"
	deviceMetricSplit  = "_"
	deviceMetricPrefix = "dev" + deviceMetricSplit
)

var (
	/*
		    From: https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt

			The "low" level means that the system is reclaiming memory for new
			allocations. Monitoring this reclaiming activity might be useful for
			maintaining cache level. Upon notification, the program (typically
			"Activity Manager") might analyze vmstat and act in advance (i.e.
			prematurely shutdown unimportant services).

			The "medium" level means that the system is experiencing medium memory
			pressure, the system might be making swap, paging out active file caches,
			etc. Upon this event applications may decide to further analyze
			vmstat/zoneinfo/memcg or internal memory usage statistics and free any
			resources that can be easily reconstructed or re-read from a disk.

			The "critical" level means that the system is actively thrashing, it is
			about to out of memory (OOM) or even the in-kernel OOM killer is on its
			way to trigger. Applications should do whatever they can to help the
			system. It might be too late to consult with vmstat or any other
			statistics, so it's advisable to take an immediate action.
	*/
	pressureLevel = sets.NewString("low", "medium", "critical")
)

// ExpressionArgs group args used for expression detection
type ExpressionArgs struct {
	Expression      string         `json:"expression"`
	WarningCount    int            `json:"warning_count"`
	WarningDuration times.Duration `json:"warning_duration"`
}

// EWMAArgs group args used for ewma detection
type EWMAArgs struct {
	Metric string `json:"metric"`
	Nr     int    `json:"nr"`
}

// HealthCheckConfig is the config for checking health, such as node load or online job interference
type HealthCheckConfig struct {
	Disable      bool         `json:"disable"`
	RuleNodes    []string     `json:"rule_nodes"`
	RuleCheck    RuleCheck    `json:"rule_check"`
	CgroupNotify NotifyConfig `json:"cgroup_notify"`
	// assign the value when initialize
	PredictReserved *Resource `json:"-"`
}

// RuleCheck group all rules
type RuleCheck struct {
	ContainerRules []*RuleCheckConfig `json:"container_rules"`
	NodeRules      []*RuleCheckConfig `json:"node_rules"`
	AppRules       []*RuleCheckConfig `json:"app_rules"`
}

// RuleCheckConfig define the rule config
type RuleCheckConfig struct {
	Name    string   `json:"name"`
	Metrics []string `json:"metrics"`
	// CheckInterval describes the interval to trigger detection
	CheckInterval times.Duration `json:"check_interval"`
	// HandleInterval describes the interval to handle conflicts after detecting abnormal result
	HandleInterval times.Duration `json:"handle_interval"`
	// RecoverInterval describes the interval to recover conflicts after detecting normal result
	RecoverInterval times.Duration        `json:"recover_interval"`
	Rules           []*DetectActionConfig `json:"rules"`
	RecoverRules    []*DetectActionConfig `json:"recover_rules"`
}

// DetectActionConfig define detectors and actions
type DetectActionConfig struct {
	Detects []*DetectConfig `json:"detects"`
	Actions []*ActionConfig `json:"actions"`
}

// DetectConfig define detector config
type DetectConfig struct {
	Name    string          `json:"name"`
	ArgsStr json.RawMessage `json:"args"`
	Args    interface{}     `json:"-"`
}

// ActionConfig define action config
type ActionConfig struct {
	Name    string          `json:"name"`
	ArgsStr json.RawMessage `json:"args"`
	Args    interface{}     `json:"-"`
}

// NotifyConfig monitor resource by kernel notify
type NotifyConfig struct {
	MemoryCgroup *MemoryNotifyConfig `json:"memory_cgroup"`
}

// MemoryNotifyConfig describe memory cgroup notify
type MemoryNotifyConfig struct {
	Pressures []MemoryPressureNotifyConfig `json:"pressures"`
	Usages    []MemoryUsageNotifyConfig    `json:"usages"`
}

// MemoryPressureNotifyConfig describe memory.pressure_level notify data
type MemoryPressureNotifyConfig struct {
	Cgroups       []string `json:"cgroups"`
	PressureLevel string   `json:"pressure_level"`
	// assign time duration the pressure has kept
	Duration times.Duration `json:"duration"`
	// assign event number in the duration time
	Count int `json:"count"`
}

// MemoryUsageNotifyConfig describe memory.usage_in_bytes notify data
type MemoryUsageNotifyConfig struct {
	Cgroups []string `json:"cgroups"`
	// the distance between limit and threshold
	MarginMb int `json:"margin_mb"`
	// when to handle event after receiving event
	Duration times.Duration `json:"duration"`
}

// InitHealthCheckConfigFunc return function to get health check config
func InitHealthCheckConfigFunc(nodeMetrics *MetricsNodeConfig,
	predictReserved *Resource) func(string) (*HealthCheckConfig, error) {
	return func(configFile string) (*HealthCheckConfig, error) {
		config := HealthCheckConfig{}
		bytes, err := ioutil.ReadFile(configFile)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(bytes, &config)
		if err != nil {
			return nil, err
		}
		config.PredictReserved = predictReserved

		for _, rule := range config.RuleCheck.NodeRules {
			ruleCheckAvailable(rule, nodeMetrics, predictReserved)
		}
		for _, rule := range config.RuleCheck.ContainerRules {
			ruleCheckAvailable(rule, nodeMetrics, predictReserved)
		}
		for _, rule := range config.RuleCheck.AppRules {
			ruleCheckAvailable(rule, nodeMetrics, predictReserved)
		}
		initNotifyConfig(&config.CgroupNotify)
		return &config, nil
	}
}

func ruleCheckAvailable(ruleCheck *RuleCheckConfig, nodeMetrics *MetricsNodeConfig, predictReserve *Resource) {
	if ruleCheck == nil {
		return
	}

	if ruleCheck.CheckInterval.Seconds() == 0 {
		ruleCheck.CheckInterval = defaultRuleInterval
	}
	if ruleCheck.HandleInterval.Seconds() == 0 {
		ruleCheck.HandleInterval = ruleCheck.CheckInterval
	}
	if ruleCheck.RecoverInterval.Seconds() == 0 {
		ruleCheck.RecoverInterval = ruleCheck.CheckInterval
	}

	for _, detectAc := range ruleCheck.Rules {
		detectActionAvailable(ruleCheck, detectAc, nodeMetrics, predictReserve)
	}
	for _, detectAc := range ruleCheck.RecoverRules {
		detectActionAvailable(ruleCheck, detectAc, nodeMetrics, predictReserve)
	}
}

func detectActionAvailable(ruleCheck *RuleCheckConfig, detectAction *DetectActionConfig,
	nodeMetrics *MetricsNodeConfig, predictReserve *Resource) {
	for _, detector := range detectAction.Detects {
		switch detector.Name {
		case DetectionEWMA:
			args := &EWMAArgs{}
			err := json.Unmarshal(detector.ArgsStr, args)
			if err != nil {
				klog.Fatalf("invalid %s rule(%s): %v", detector.Name, detector.ArgsStr, err)
			}
			detector.Args = args
		case DetectionExpression:
			if len(ruleCheck.Metrics) == 0 {
				klog.Fatalf("rule %s should specific metric", detector.Name)
			}

			args := &ExpressionArgs{}
			err := json.Unmarshal(detector.ArgsStr, args)
			if err != nil {
				klog.Fatalf("invalid %s rule(%s): %v", detector.Name, detector.ArgsStr, err)
			}
			if args.WarningCount <= 0 && args.WarningDuration == 0 {
				args.WarningCount = defaultExpressionWarningCount
			}
			detector.Args = args

			initExpressionEvaluate(ruleCheck, detector, nodeMetrics, predictReserve)
		}
	}
}

func initExpressionEvaluate(ruleCheck *RuleCheckConfig, detector *DetectConfig, nodeMetrics *MetricsNodeConfig,
	predictReserve *Resource) {
	args := detector.Args.(*ExpressionArgs)
	total, _ := machine.GetTotalResource()
	switch ruleCheck.Name {
	case ruleNameCpu:
		// auto generated monitor expression
		if args.Expression == ExpresstionAutoDetect {
			metric := ruleCheck.Metrics[0]
			cpuReserve := *predictReserve.CpuMilli
			cpuTotal := float64(total.Cpu().MilliValue())
			cpuUsage := (cpuTotal - cpuReserve) / cpuTotal
			if cpuUsage < 0.85 {
				cpuUsage += 0.05
			}
			args.Expression = fmt.Sprintf("%s > %f", metric, cpuUsage)
			klog.V(2).Infof("cpu monitor expression auto detected: %s", args.Expression)
		}
	case ruleNameMemmory:
		// auto generated monitor expression
		if args.Expression == ExpresstionAutoDetect {
			metric := ruleCheck.Metrics[0]
			memReserve := *predictReserve.MemMB
			memAvailable := memReserve*float64(MemUnit) - float64(1024*MemUnit)
			args.Expression = fmt.Sprintf("%s < %f", metric, memAvailable)
			klog.V(2).Infof("memory monitor expression auto detected: %s", args.Expression)
		}
	case ruleNameDiskIO:
		ruleCheck.Metrics = generateMetricsWithDev(ruleCheck.Metrics, nodeMetrics.DiskNames)
		args.Expression = generateExpressionEvaluateWithDev(args.Expression, nodeMetrics.DiskNames)
	case ruleNameNetIO:
		ruleCheck.Metrics = generateMetricsWithDev(ruleCheck.Metrics, nodeMetrics.Ifaces)
		args.Expression = generateExpressionEvaluateWithDev(args.Expression, nodeMetrics.Ifaces)
	}
}

func generateMetricsWithDev(metrics, devices []string) []string {
	var newMetrics []string
	for _, m := range metrics {
		if !strings.Contains(m, genericDev) {
			newMetrics = append(newMetrics, m)
		} else {
			for _, dev := range devices {
				newDev := deviceMetricPrefix + dev
				newMetrics = append(newMetrics, strings.Replace(m, genericDev, newDev, -1))
			}
		}
	}

	return newMetrics
}

// GetDeviceNameFromMetric parse the metric name, and output the dev and devMetric name
func GetDeviceNameFromMetric(metric string) (dev, devMetric, originalMetric string) {
	if !strings.HasPrefix(metric, deviceMetricPrefix) {
		return "", metric, metric
	}

	devMetric = strings.TrimPrefix(metric, deviceMetricPrefix)
	index := strings.Index(devMetric, "_")
	dev = devMetric[:index]
	originalMetric = devMetric[index+1:]
	return dev, devMetric, originalMetric
}

func generateExpressionEvaluateWithDev(expStr string, devices []string) string {
	if !strings.Contains(expStr, genericDev) || len(devices) == 0 {
		return expStr
	}
	var newExpStrs []string
	for _, dev := range devices {
		newExpStrs = append(newExpStrs, strings.Replace(expStr, genericDev, dev, -1))
	}

	return strings.Join(newExpStrs, " || ")
}

func initNotifyConfig(cfg *NotifyConfig) {
	if cfg.MemoryCgroup != nil {
		for _, pressureCfg := range cfg.MemoryCgroup.Pressures {
			if len(pressureCfg.Cgroups) != 0 && !pressureLevel.Has(pressureCfg.PressureLevel) {
				klog.Fatalf("invalid pressure level, must be one of %v", pressureLevel)
			}
		}
	}
}
