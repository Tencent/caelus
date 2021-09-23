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

package metricadapter

import "time"

const (
	defaultMetricName  = "latency"
	defaultServingPort = 9103
)

type podMetric struct {
	PodName    string  `json:"pod_name"`
	Namespace  string  `json:"namespace"`
	MetricName string  `json:"metric_name"`
	Slo        float64 `json:"slo"`
	Value      float64 `json:"value"`

	LastUpdataTime time.Time `json:"last_updata_time"`
}

type podMetrics struct {
	Data []podMetric `json:"data"`
}

// pod meta info
type metricMeta struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	MetricName string `json:"metric_name"`
}

// AdapterConfig group config of multiple adapters
type AdapterConfig struct {
	// app metrics server address
	ServingPort int `json:"serving_port"`
}
