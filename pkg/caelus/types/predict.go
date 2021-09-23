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
	"fmt"
	"math"
	"time"

	"github.com/tencent/caelus/pkg/caelus/util/machine"
	"github.com/tencent/caelus/pkg/util/times"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/model"
	"k8s.io/klog"
)

const (
	// LocalPredictorType is the local predictor
	LocalPredictorType = "local"
	// VPAPredictorType is the remote VPA predictor
	VPAPredictorType = "vpa"

	defaultPredictCheckInterval = time.Second * 30
	defaultPredictPrintInterval = time.Minute * 5

	defaultAnomalyDetectorMovingWindow = time.Minute * 10

	NodeResourceTypeOnlinePredict = "online_predict"

	defaultPodMinCPUMillicores   = 10.0
	defaultPodMinMemoryMb        = 10.0
	defaultSafetyMarginFraction  = 0.08
	defaultCPUPercentile         = 0.8
	defaultMemoryPeaksPercentile = 0.9
)

var (
	AvailablePredictType      = sets.NewString(LocalPredictorType, VPAPredictorType)
	AvailableLocalPredictType = sets.NewString(LocalPredictorType)
)

// AggregationsConfig is used to configure aggregation behaviour.
type AggregationsConfig struct {
	// MemoryAggregationInterval is the length of a single interval, for
	// which the peak memory usage is computed.
	// Memory usage peaks are aggregated in multiples of this interval. In other words
	// there is one memory usage sample per interval (the maximum usage over that
	// interval).
	MemoryAggregationInterval times.Duration `json:"memory_aggregation_interval"`
	// MemoryAggregationWindowIntervalCount is the number of consecutive MemoryAggregationIntervals
	// which make up the MemoryAggregationWindowLength which in turn is the period for memory
	// usage aggregation by VPA.
	MemoryAggregationIntervalCount int64 `json:"memory_aggregation_interval_count"`
	// MemoryHistogramDecayHalfLife is the amount of time it takes a historical
	// memory usage sample to lose half of its weight. In other words, a fresh
	// usage sample is twice as 'important' as one with age equal to the half
	// life period.
	MemoryHistogramDecayHalfLife times.Duration `json:"memory_histogram_decay_half_life"`
	// CPUHistogramDecayHalfLife is the amount of time it takes a historical
	// CPU usage sample to lose half of its weight.
	CPUHistogramDecayHalfLife times.Duration `json:"cpu_histogram_decay_half_life"`
}

// PredictConfig group options for predictor
type PredictConfig struct {
	Disable       bool           `json:"disable"`
	CheckInterval times.Duration `json:"check_interval"`
	// PredictType must in [local, localv2, vpa]
	PredictType       string   `json:"predict_type"`
	PredictServerAddr string   `json:"predict_server_addr"`
	ReserveResource   Resource `json:"reserve_resource"`
	// PrintInterval is the the time interval to print predict detailed log for debug
	PrintInterval times.Duration `json:"print_interval"`
	// LocalPredictConfig is the configuration for local predictor
	LocalPredictConfig `json:",inline"`
	// The type value of online predict metrics caelus_node_resource{type=""}
	// It's used by experiment predict
	PredictMetricsType string `json:"predict_metrics_type"`
}

// LocalPredictConfig group options for local predictor
type LocalPredictConfig struct {
	// Minimum CPU recommendation for a pod
	PodMinCPUMillicores float64 `json:"pod_min_cpu_millicores"`
	// Minimum memory recommendation for a pod
	PodMinMemoryMb float64 `json:"pod_min_memory_mb"`
	// Fraction of usage added as the safety margin to the recommended request
	SafetyMarginFraction float64 `json:"safety_margin_fraction"`
	// cpu usage percentile to recommend cpu resource
	CPUPercentile float64 `json:"cpu_percentile"`
	// memory usage percentile to recommend cpu resource
	MemoryPeaksPercentile float64 `json:"memory_peaks_percentile"`
	// AggregationsConfig is used to configure aggregation behaviour.
	AggregationsConfig `json:",inline"`
	// Enable tune cpu weight if cpu usage is anomaly
	EnableTuneCPUWeight bool `json:"enable_tune_cpu_weight"`
	// AnomalyDetectorMovingWindow defines how long the moving window of anomaly detector should keep
	AnomalyDetectorMovingWindow times.Duration `json:"anomaly_detector_moving_window"`
	// If detect cpu usage increasing anomaly, the weight of the anomaly sample
	// Base weight is 100
	IncreasingAnomalyWeightFactor int64 `json:"increasing_anomaly_weight_factor"`
	// If detect cpu usage decreasing anomaly, the weight of the anomaly sample
	// Base weight is 100
	DecreasingAnomalyWeightFactor int64 `json:"decreasing_anomaly_weight_factor"`
}

// InitPredictConfig validate and format predict config
func InitPredictConfig(config *PredictConfig) {
	// If predict is disabled, the predict type must be local.
	if config.Disable {
		config.PredictType = LocalPredictorType
	}
	// assign default predict type
	if len(config.PredictType) == 0 {
		config.PredictType = LocalPredictorType
	}
	if !AvailablePredictType.Has(config.PredictType) {
		klog.Fatalf("invalid predict type: %s, must be %v", config.PredictType, AvailablePredictType)
	}
	if config.PredictType == VPAPredictorType && len(config.PredictServerAddr) == 0 {
		klog.Fatalf("predict server address must be assign when type is vpa")
	}
	if AvailableLocalPredictType.Has(config.PredictType) {
		checkLocalPredictType(config)
	}

	if len(config.ReserveResource.CpuPercentStr) != 0 {
		cpuPercent, err := parsePercentage(config.ReserveResource.CpuPercentStr)
		if err != nil {
			klog.Fatalf("invalid predict reserved resource: %s", config.ReserveResource.CpuPercentStr)
		}
		config.ReserveResource.CpuPercent = &cpuPercent
	}
	if len(config.ReserveResource.MemPercentStr) != 0 {
		memPercent, err := parsePercentage(config.ReserveResource.MemPercentStr)
		if err != nil {
			klog.Fatalf("invalid predict reserved resource: %s", config.ReserveResource.MemPercentStr)
		}
		config.ReserveResource.MemPercent = &memPercent
	}

	err := getReservedResource(&config.ReserveResource)
	if err != nil {
		klog.Fatalf("get predict reserved resource failed: %v", err)
	}
}

func checkLocalPredictType(config *PredictConfig) {
	if config.MemoryAggregationInterval.Seconds() == 0 {
		config.MemoryAggregationInterval = times.Duration(model.DefaultMemoryAggregationInterval)
	}
	if config.MemoryAggregationIntervalCount == 0 {
		config.MemoryAggregationIntervalCount = model.DefaultMemoryAggregationIntervalCount
	}
	if config.CPUHistogramDecayHalfLife.Seconds() == 0 {
		config.CPUHistogramDecayHalfLife = times.Duration(model.DefaultCPUHistogramDecayHalfLife)
	}
	if config.MemoryHistogramDecayHalfLife.Seconds() == 0 {
		config.MemoryHistogramDecayHalfLife = times.Duration(model.DefaultMemoryHistogramDecayHalfLife)
	}
	if config.CheckInterval.Seconds() == 0 {
		config.CheckInterval = times.Duration(defaultPredictCheckInterval)
	}
	if config.PrintInterval.Seconds() == 0 {
		config.PrintInterval = times.Duration(defaultPredictPrintInterval)
	}
	if config.PodMinCPUMillicores == 0 {
		config.PodMinCPUMillicores = defaultPodMinCPUMillicores
	}
	if config.PodMinMemoryMb == 0 {
		config.PodMinMemoryMb = defaultPodMinMemoryMb
	}
	if config.SafetyMarginFraction == 0 {
		config.SafetyMarginFraction = defaultSafetyMarginFraction
	}
	if config.CPUPercentile == 0 {
		config.CPUPercentile = defaultCPUPercentile
	}
	if config.MemoryPeaksPercentile == 0 {
		config.MemoryPeaksPercentile = defaultMemoryPeaksPercentile
	}
	if config.EnableTuneCPUWeight && config.AnomalyDetectorMovingWindow.Seconds() == 0 {
		config.AnomalyDetectorMovingWindow = times.Duration(defaultAnomalyDetectorMovingWindow)
	}
	if config.IncreasingAnomalyWeightFactor == 0 {
		config.IncreasingAnomalyWeightFactor = 1
	}
	if config.DecreasingAnomalyWeightFactor == 0 {
		config.DecreasingAnomalyWeightFactor = 1
	}
}

func getReservedResource(reserved *Resource) error {
	total, err := machine.GetTotalResource()
	if err != nil {
		return err
	}

	// get memory reserved resource
	memTotal := float64(total.Memory().Value() / MemUnit)
	memReserved := getReservedValue(memTotal, reserved.MemPercent, reserved.MemMB)
	reserved.MemMB = &memReserved

	// get cpu reserved resource
	cpuTotal := float64(total.Cpu().MilliValue())
	cpuReserved := getReservedValue(cpuTotal, reserved.CpuPercent, reserved.CpuMilli)
	reserved.CpuMilli = &cpuReserved

	return nil
}

// getReservedValue compare percent and value, and return the min value
func getReservedValue(total float64, percent, value *float64) float64 {
	if percent == nil && value == nil {
		return float64(0)
	}
	if percent == nil {
		return *value
	}
	if value == nil {
		return total * (*percent)
	}

	// Get the min value between percent value and assigned value
	return math.Min(total*(*percent), *value)
}

// initPredictConfigs validate and format all predict configs
func initPredictConfigs(configs []PredictConfig) {
	if len(configs) == 0 {
		klog.Fatalf("empty predict configs")
	}
	for i := range configs {
		if i == 0 {
			configs[i].PredictMetricsType = NodeResourceTypeOnlinePredict
		} else if configs[i].PredictMetricsType == "" {
			configs[i].PredictMetricsType = fmt.Sprintf("experiment_%s_%d", NodeResourceTypeOnlinePredict, i)
		}
		InitPredictConfig(&configs[i])
	}
}
