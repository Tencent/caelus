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

import (
	"encoding/json"
	store "github.com/google/cadvisor/utils"
	"github.com/tencent/caelus/pkg/util/times"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/wait"
	"net/http"
	"sort"
	"sync"
	"time"
)

// server_adapter setup a server, application can report metrics by /api/latency

const (
	MaxItem = 1000000
)

type serverAdapter struct {
	detailLock        sync.RWMutex
	metricDetailCache map[metricMeta]*detailData

	sloLock        sync.RWMutex
	metricSloCache map[metricMeta]podMetric
}

var _ adapter = (*serverAdapter)(nil)

func newServerAdapter() *serverAdapter {
	adapter := &serverAdapter{
		metricDetailCache: make(map[metricMeta]*detailData),
		metricSloCache:    make(map[metricMeta]podMetric),
	}
	return adapter
}

// Result is the restful format
type Result struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ReportDetailData group latency data reported by workload
type ReportDetailData struct {
	metricMeta `json:",inline"`
	// raw single latency value
	// target latency value
	SLO float64 `json:"slo"`
	// avg or percentile value
	SLI      float64        `json:"sli"`
	Duration times.Duration `json:"duration"`
	Data     []float64      `json:"data"`
}

type detailData struct {
	SLO      float64        `json:"slo"`
	SLI      float64        `json:"sli"`
	Duration times.Duration `json:"duration"`
	*store.TimedStore
}

// ReportSloData represent latency data reported by workload
type ReportSloData struct {
	metricMeta `json:",inline"`
	// raw single latency value
	// target latency value
	SLO   float64 `json:"slo"`
	Value float64 `json:"value"`
}

// Run start to tun server adapter
func (adapter *serverAdapter) Run(stopCh <-chan struct{}) {
	http.HandleFunc("/appmetrics", adapter.reportDetail)
	http.HandleFunc("/appslo", adapter.reportSlo)
	go wait.Until(adapter.gc, time.Minute, stopCh)
}

// reportDetail handle app metrics reported by workload
func (adapter *serverAdapter) reportDetail(w http.ResponseWriter, r *http.Request) {
	res := Result{}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		res.Code = -1
		res.Message = err.Error()
		resp, _ := json.Marshal(res)
		w.Write(resp)
		return
	}
	data := ReportDetailData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		res.Code = -1
		res.Message = err.Error()
		resp, _ := json.Marshal(res)
		w.Write(resp)
		return
	}
	if data.Name == "" || data.Namespace == "" {
		res.Code = -1
		res.Message = "query parameters name and namespace must be specified"
		resp, _ := json.Marshal(res)
		w.Write(resp)
		return
	}
	if data.Duration == 0 {
		data.Duration = times.Duration(15 * time.Second)
	}
	if data.Duration > times.Duration(time.Minute) {
		data.Duration = times.Duration(time.Minute)
	}
	if data.MetricName == "" {
		data.MetricName = defaultMetricName
	}
	if data.SLI < 50 {
		data.SLI = 90
	}
	// TODO: store
	resp, _ := json.Marshal(res)
	w.Write(resp)
}

// addDetail add or update new report metric data
func (adapter *serverAdapter) addDetail(data *ReportDetailData) {
	adapter.detailLock.Lock()
	defer adapter.detailLock.Unlock()
	dd, exist := adapter.metricDetailCache[data.metricMeta]
	if !exist {
		dd = &detailData{
			TimedStore: store.NewTimedStore(time.Minute, MaxItem),
		}
	}
	dd.SLI = data.SLI
	dd.SLO = data.SLO
	dd.Duration = data.Duration
	now := time.Now()
	dd.Add(now, data.Data)
}

// aggregateDetail store metrics slo data
func (adapter *serverAdapter) aggregateDetail() {
	adapter.detailLock.Lock()
	defer adapter.detailLock.Unlock()
	sloCache := make(map[metricMeta]podMetric)
	now := time.Now()
	for meta, dd := range adapter.metricDetailCache {
		startTime := now.Add(-dd.Duration.TimeDuration())
		datas := dd.InTimeRange(startTime, now, MaxItem)
		var nums []float64
		for _, data := range datas {
			fdata := data.([]float64)
			nums = append(nums, fdata...)
		}
		if len(nums) == 0 {
			delete(adapter.metricDetailCache, meta)
			continue
		}
		sort.Float64s(nums)
		pIdx := int(float64(len(nums))*dd.SLI)/100 - 1
		if pIdx < 0 {
			pIdx = 0
		}
		val := nums[pIdx]
		sloCache[meta] = podMetric{
			PodName:        meta.Name,
			Namespace:      meta.Namespace,
			MetricName:     meta.MetricName,
			Slo:            dd.SLO,
			Value:          val,
			LastUpdataTime: now,
		}
	}

	adapter.sloLock.Lock()
	for meta, metric := range sloCache {
		adapter.metricSloCache[meta] = metric
	}
	adapter.sloLock.Unlock()
}

// reportSlo handle app slo metrics reported by workload
func (adapter *serverAdapter) reportSlo(w http.ResponseWriter, r *http.Request) {
	res := Result{}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		res.Code = -1
		res.Message = err.Error()
		resp, _ := json.Marshal(res)
		w.Write(resp)
		return
	}
	data := ReportSloData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		res.Code = -1
		res.Message = err.Error()
		resp, _ := json.Marshal(res)
		w.Write(resp)
		return
	}
	adapter.sloLock.Lock()
	defer adapter.sloLock.Unlock()
	meta := data.metricMeta
	adapter.metricSloCache[meta] = podMetric{
		PodName:        meta.Name,
		Namespace:      meta.Namespace,
		MetricName:     meta.MetricName,
		Slo:            data.SLO,
		Value:          data.Value,
		LastUpdataTime: time.Now(),
	}
}

// GetSlo return pod slo metrics
func (adapter *serverAdapter) GetSlo() []podMetric {
	var slos []podMetric
	adapter.sloLock.RLock()
	for _, slo := range adapter.metricSloCache {
		if time.Since(slo.LastUpdataTime) < time.Minute {
			slos = append(slos, slo)
		}
	}
	adapter.sloLock.RUnlock()
	return slos
}

// gc delete useless data
func (adapter *serverAdapter) gc() {
	adapter.sloLock.Lock()
	for meta, metric := range adapter.metricSloCache {
		if time.Since(metric.LastUpdataTime) > time.Minute {
			delete(adapter.metricSloCache, meta)
		}
	}
	adapter.sloLock.Unlock()
}
