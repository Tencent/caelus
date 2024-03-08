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

package serverrequest

import (
	"github.com/tencent/caelus/pkg/caelus/metrics/outer"
	"github.com/tencent/caelus/pkg/caelus/statestore"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/klog/v2"
)

const (
	metricsServerRequest     = "caelus_server_request_collector"
	metricsServerRequestDesc = "external metrics from server request"
	metricPrefix             = "caelus_"
)

type requestMetrics struct {
	desc    *prometheus.Desc
	stStore statestore.StateStore
}

func newRequestMetrics(stStore statestore.StateStore) (prometheus.Collector, error) {
	desc := prometheus.NewDesc(metricsServerRequest,
		metricsServerRequestDesc, []string{"node"}, nil)

	return &requestMetrics{
		desc:    desc,
		stStore: stStore,
	}, nil
}

// RegisterRequestMetrics registers external metrics from server request
func RegisterRequestMetrics(reg *prometheus.Registry, store statestore.StateStore) {
	if metrics, err := newRequestMetrics(store); err == nil {
		reg.MustRegister(metrics)
		klog.Infof("Register server request metrics successfully")
	} else {
		klog.Fatalf("Register server request metrics failed, err: %v", err)
	}
}

// Collect implements the prometheus.Collector interface.
func (o *requestMetrics) Collect(ch chan<- prometheus.Metric) {
	metricFamilies, err := o.stStore.GetPromDirectMetrics()
	if err != nil {
		klog.Errorf("fetch prometheus metrics failed: %v", err)
		return
	}
	o.handleMetrics(metricFamilies, ch)
}

// Describe puts metrics desc to prometheus.Desc chan
func (o *requestMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- o.desc
}

func (o *requestMetrics) handleMetrics(metricFamilies map[string]*dto.MetricFamily, ch chan<- prometheus.Metric) {
	for _, mf := range metricFamilies {
		// avoid duplication with the original metric
		if mf.Name != nil {
			newName := metricPrefix + *mf.Name
			mf.Name = &newName
		}

		for _, metric := range mf.Metric {
			var values, names []string
			for _, label := range metric.GetLabel() {
				names = append(names, label.GetName())
				values = append(values, label.GetValue())
			}
			utils.HandleMetricType(mf, metric, values, names, ch)
		}
	}
}
