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
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	cadvisor "github.com/google/cadvisor/utils"
	"github.com/parnurzeal/gorequest"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	customizeResourceName = "Customize"
	defaultMetricName     = "latency"
)

// CustomizeStoreInterface describe special customize state interface
type CustomizeStoreInterface interface {
	GetCustomizeResourceRecentState(jobName, metricName string) (*Metric, error)
	GetCustomizeResourceRangeStats(jobName, metricName string, start, end time.Time, count int) ([]*Metric, error)
	ListCustomizeResourceRecentState() (map[string]*Metric, error)
	ListCustomizeResourceRangeStats(start, end time.Time, count int) (map[string][]*Metric, error)
}

// StoreInterfaceForTest for testing
type StoreInterfaceForTest interface {
	CustomizeStoreInterface
	AddMetricsData(key string, now time.Time, metric Metric)
}

// CustomizeStore describe common state interface inherent from state store
type CustomizeStore interface {
	CustomizeStoreInterface
	Name() string
	Run(stop <-chan struct{})
}

type customizeStoreManager struct {
	*types.OnlineConfig
	lock   sync.RWMutex
	caches map[string]*cadvisor.TimedStore
	maxAge time.Duration
}

// NewCustomizeStoreManager new a customize store manager instance
func NewCustomizeStoreManager(cacheTTL time.Duration, config *types.OnlineConfig) CustomizeStore {
	return &customizeStoreManager{
		OnlineConfig: config,
		caches:       make(map[string]*cadvisor.TimedStore),
		maxAge:       cacheTTL,
	}
}

// Name show module name
func (c *customizeStoreManager) Name() string {
	return customizeResourceName
}

// Run call main loop
func (c *customizeStoreManager) Run(stop <-chan struct{}) {
	c.collect(stop)
}

// GetCustomizeResourceRecentState get newest customize state data for the online job with metric name
func (c *customizeStoreManager) GetCustomizeResourceRecentState(jobName, metricName string) (*Metric, error) {
	key := generateStoreKey(jobName, metricName)

	c.lock.RLock()
	defer c.lock.RUnlock()
	cache, ok := c.caches[key]
	if !ok {
		klog.Errorf("customize no metric name found: %s", key)
		return nil, util.ErrNotFound
	}

	m := cache.Get(0)
	return m.(*Metric), nil
}

// GetCustomizeResourceRangeStats get customize state data in period for online job with metric name
func (c *customizeStoreManager) GetCustomizeResourceRangeStats(jobName, metricName string,
	start, end time.Time, count int) ([]*Metric, error) {
	var metrics []*Metric

	c.lock.RLock()
	defer c.lock.RUnlock()
	key := generateStoreKey(jobName, metricName)
	cache, ok := c.caches[key]
	if !ok {
		klog.Errorf("customize no metric name found: %s", key)
		return metrics, util.ErrNotFound
	}
	datas := cache.InTimeRange(start, end, count)
	for _, d := range datas {
		metrics = append(metrics, d.(*Metric))
	}

	return metrics, nil
}

// ListCustomizeResourceRecentState list newest state data for all online jobs
func (c *customizeStoreManager) ListCustomizeResourceRecentState() (map[string]*Metric, error) {
	metrics := make(map[string]*Metric)

	c.lock.RLock()
	defer c.lock.RUnlock()
	for k, cache := range c.caches {
		metrics[k] = cache.Get(0).(*Metric)
	}

	return metrics, nil
}

// ListCustomizeResourceRangeStats list state data in period for all online jobs
func (c *customizeStoreManager) ListCustomizeResourceRangeStats(start, end time.Time,
	count int) (map[string][]*Metric, error) {
	metrics := make(map[string][]*Metric)

	c.lock.RLock()
	defer c.lock.RUnlock()
	for k, cache := range c.caches {
		var values []*Metric
		ms := cache.InTimeRange(start, end, count)
		if len(ms) == 0 {
			continue
		}
		for _, m := range ms {
			values = append(values, m.(*Metric))
		}
		metrics[k] = values
	}

	return metrics, nil
}

// collect main loop to collect online metrics data
func (c *customizeStoreManager) collect(stop <-chan struct{}) {
	for i := range c.Jobs {
		job := c.Jobs[i]
		for j := range job.Metrics {
			metric := job.Metrics[j]
			go wait.Until(func() {
				var datas *MetricsResponseData
				// metric data from local command or remote server
				if len(metric.Source.MetricsCommand) != 0 {
					datas = getMetricsFromCommand(metric.Source.MetricsCommand, *metric.Source.CmdNeedChroot)
				} else if len(metric.Source.MetricsURL) != 0 {
					datas = getMetricsFromURL(metric.Source.MetricsURL)
				}
				if datas == nil {
					return
				}
				if datas.Code != 0 {
					klog.Errorf("get metrics from %+v, response not ok(%d): %s",
						metric.Source, datas.Code, datas.Msg)
					return
				}
				now := time.Now()

				for _, data := range datas.Data {
					jobName := data.JobName
					metricName := data.MetricName
					if len(jobName) == 0 {
						jobName = job.Name
					}
					if len(metricName) == 0 {
						metricName = metric.Name
					}
					if len(jobName) == 0 || len(metricName) == 0 {
						klog.Errorf("no job or metric name when request metrics found from %+v", metric.Source)
					}
					key := generateStoreKey(jobName, metricName)
					c.AddMetricsData(key, now, Metric{JobName: key, Values: data.Values, Ts: now})
				}
			}, metric.Source.CheckInterval.TimeDuration(), stop)
		}
	}
	if c.CustomMetric.MetricServerAddr == "" {
		return
	}
	go wait.Until(func() {
		request := gorequest.New()
		resp, body, errs := request.Get(c.CustomMetric.MetricServerAddr).End()
		if errs != nil {
			klog.V(4).Infof("failed fetch metric")
			return
		}
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("status code is %v", resp.StatusCode)
			return
		}

		var res podMetrics
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			klog.Errorf("failed to unmarshal %v: %v", body, err)
			return
		}
		now := time.Now()
		for _, r := range res.Data {
			key := generatePodStoreKey(r.Namespace, r.PodName, r.MetricName)
			c.AddMetricsData(key, now, Metric{JobName: key, Values: map[string]float64{"value": r.Value, "slo": r.Slo}})
		}

	}, c.CustomMetric.CollectInterval.TimeDuration(), stop)
}

// AddMetricsData add metric by key, expose public for test usage
func (c *customizeStoreManager) AddMetricsData(key string, now time.Time, metric Metric) {
	var cache *cadvisor.TimedStore

	func() {
		var ok bool
		c.lock.Lock()
		defer c.lock.Unlock()

		cache, ok = c.caches[key]
		if !ok {
			cache = cadvisor.NewTimedStore(c.maxAge, -1)
			c.caches[key] = cache
		}
	}()

	cache.Add(now, &metric)
}

func (c *customizeStoreManager) delMetricsData() {
	c.lock.Lock()
	defer c.lock.Unlock()

	for m, cache := range c.caches {
		// get last element
		stats := cache.InTimeRange(time.Now().Add(-10*time.Minute), time.Now(), -1)
		if len(stats) == 0 {
			klog.Warningf("delete job customize(%s) cache for time expired", m)
			delete(c.caches, m)
		}
	}
}

func generateStoreKey(jobName, metricName string) string {
	if metricName == "" {
		metricName = defaultMetricName
	}
	return fmt.Sprintf("%s-%s", jobName, metricName)
}

func generatePodStoreKey(namespace, podName, metricName string) string {
	return generateStoreKey(fmt.Sprintf("%s/%s", namespace, podName), metricName)
}

// getMetricsFromCommand get metric data from local command
func getMetricsFromCommand(metricsCmd []string, needChroot bool) *MetricsResponseData {
	var data = &MetricsResponseData{}
	var cmd *exec.Cmd

	if needChroot {
		args := []string{types.RootFS}
		args = append(args, metricsCmd...)
		cmd = exec.Command("chroot", args...)
	} else {
		cmd = exec.Command(metricsCmd[0], metricsCmd[1:]...)
	}
	out, err := cmd.Output()
	if err != nil {
		klog.Errorf("execute command(%s) err: %v", metricsCmd, err)
		return nil
	}

	err = json.Unmarshal(out, data)
	if err != nil {
		klog.Errorf("unmarshal metrics command return data err: %v, outputs: %s", err, string(out))
		return nil
	}
	klog.V(2).Infof("collect metric: %s", string(out))
	return data
}

// getMetricsFromURL get metric data from remote server
func getMetricsFromURL(metricsURL string) *MetricsResponseData {
	var data = &MetricsResponseData{}
	rsp, rspBytes, errs := gorequest.New().Timeout(time.Minute).Get(metricsURL).EndBytes()

	if len(errs) > 0 {
		klog.Errorf("request metrics from %s err: %v", metricsURL, errs)
		return nil
	}
	if rsp.StatusCode != http.StatusOK {
		klog.Errorf("request metrics from %s, status code not ok: %d", metricsURL, rsp.StatusCode)
		return nil
	}
	err := json.Unmarshal(rspBytes, data)
	if err != nil {
		klog.Errorf("unmarshal metrics url data err: %v, response data: %s", err, string(rspBytes))
		return nil
	}

	return data
}
