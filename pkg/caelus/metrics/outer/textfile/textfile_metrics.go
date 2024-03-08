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

package textfile

import (
	"github.com/prometheus/client_golang/prometheus"
	klog "k8s.io/klog/v2"
)

const (
	metricsTextfile     = "caelus_textfile_collector"
	metricsTextfileDesc = "external metrics from textfile"
)

type textFileMetrics struct {
	coll *textFileCollector
	desc *prometheus.Desc
}

// Collect implements the prometheus.Collector interface.
func (tm *textFileMetrics) Collect(ch chan<- prometheus.Metric) {
	tm.coll.Update(ch)
}

// Describe puts metrics descs to prometheus.Desc chan
func (tm *textFileMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- tm.desc
}

func newTextFileMetrics() (prometheus.Collector, error) {
	desc := prometheus.NewDesc(metricsTextfile,
		metricsTextfileDesc, []string{"node"}, nil)

	coll, err := newTextFileCollector()
	if err != nil {
		return nil, err

	}

	return &textFileMetrics{
		coll: coll,
		desc: desc,
	}, nil
}

// RegisterTextFileMetrics registers external metrics from textfile
func RegisterTextFileMetrics(reg *prometheus.Registry) {
	if metrics, err := newTextFileMetrics(); err == nil {
		reg.MustRegister(metrics)
		klog.Infof("Register text file metrics of directory successfully")
	} else {
		klog.Fatalf("Register text file metrics failed, err: %s", err)
	}
}
