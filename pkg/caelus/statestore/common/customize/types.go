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

package customizestore

import (
	"encoding/json"
	"time"
)

type podMetric struct {
	PodName    string  `json:"pod_name"`
	Namespace  string  `json:"namespace"`
	MetricName string  `json:"metric_name"`
	Slo        float64 `json:"slo"`
	Value      float64 `json:"value"`
}

type podMetrics struct {
	Data []podMetric `json:"data"`
}

// Metric describe online job's SLO and current metrics value
type Metric struct {
	JobName string
	Values  map[string]float64 `json:"values"`

	Ts time.Time `json:"-"`
}

// MetricsResponseData show online job metric response data
type MetricsResponseData struct {
	Code int                  `json:"code"`
	Msg  string               `json:"msg"`
	Data []MetricResponseList `json:"data"`
}

// MetricResponseList group online job metric data
type MetricResponseList struct {
	JobName    string `json:"job_name"`
	MetricName string `json:"metric_name"`
	Values     map[string]float64
}

// MarshalJSON, json data to string
func (m *MetricResponseList) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	tmp := make(map[string]interface{})
	tmp["job_name"] = m.JobName
	tmp["metric_name"] = m.MetricName
	for k, v := range m.Values {
		tmp[k] = v
	}
	return json.Marshal(tmp)
}

// UnmarshalJSON, string to json data
func (m *MetricResponseList) UnmarshalJSON(b []byte) error {
	tmp := make(map[string]interface{})
	err := json.Unmarshal(b, &tmp)
	if err != nil {
		return err
	}
	if m.Values == nil {
		m.Values = make(map[string]float64)
	}
	if name, ok := tmp["job_name"].(string); ok {
		m.JobName = name
	}
	if name, ok := tmp["metric_name"].(string); ok {
		m.MetricName = name
	}
	for k, v := range tmp {
		if vv, ok := v.(string); ok {
			if vv == "job_name" {
				m.JobName = vv
			}
			if vv == "metric_name" {
				m.MetricName = vv
			}
		}
		if vv, ok := v.(float64); ok {
			m.Values[k] = vv
		}
	}
	return nil
}
