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

package utils

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// HandleMetricType handle different prometheus metric types
func HandleMetricType(metricFamily *dto.MetricFamily, metric *dto.Metric, values, names []string,
	ch chan<- prometheus.Metric) {
	var valType prometheus.ValueType
	var val float64
	metricType := metricFamily.GetType()
	switch metricType {
	case dto.MetricType_COUNTER:
		valType = prometheus.CounterValue
		val = metric.Counter.GetValue()

	case dto.MetricType_GAUGE:
		valType = prometheus.GaugeValue
		val = metric.Gauge.GetValue()

	case dto.MetricType_UNTYPED:
		valType = prometheus.UntypedValue
		val = metric.Untyped.GetValue()

	case dto.MetricType_SUMMARY:
		quantiles := map[float64]float64{}
		for _, q := range metric.Summary.Quantile {
			quantiles[q.GetQuantile()] = q.GetValue()
		}
		ch <- prometheus.MustNewConstSummary(
			prometheus.NewDesc(
				*metricFamily.Name,
				metricFamily.GetHelp(),
				names, nil,
			),
			metric.Summary.GetSampleCount(),
			metric.Summary.GetSampleSum(),
			quantiles, values...,
		)
	case dto.MetricType_HISTOGRAM:
		buckets := map[float64]uint64{}
		for _, b := range metric.Histogram.Bucket {
			buckets[b.GetUpperBound()] = b.GetCumulativeCount()
		}
		ch <- prometheus.MustNewConstHistogram(
			prometheus.NewDesc(
				*metricFamily.Name,
				metricFamily.GetHelp(),
				names, nil,
			),
			metric.Histogram.GetSampleCount(),
			metric.Histogram.GetSampleSum(),
			buckets, values...,
		)
	default:
		panic("unknown metric type")
	}

	if metricType == dto.MetricType_GAUGE || metricType == dto.MetricType_COUNTER || metricType == dto.MetricType_UNTYPED {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				*metricFamily.Name,
				metricFamily.GetHelp(),
				names, nil,
			),
			valType, val, values...,
		)
	}

	return
}
