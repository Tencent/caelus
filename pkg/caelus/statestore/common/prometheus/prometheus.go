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

package promstore

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"

	cadvisor "github.com/google/cadvisor/utils"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const promResourceName = "Prometheus"

// PromStoreInterface describe prometheus special interface
type PromStoreInterface interface {
	GetPromDirectMetrics() (metricFamilies map[string]*dto.MetricFamily, err error)
	GetPromRangeMetrics(key string, start, end time.Time, count int) ([]float64, error)
	GetPromRecentMetrics(key string) (float64, error)
}

// PromStore describe common interface for prometheus resource inherit from state store
type PromStore interface {
	PromStoreInterface
	Name() string
	Run(stop <-chan struct{})
}

type promStoreManager struct {
	*types.MetricsPrometheus
	lock   sync.RWMutex
	caches map[string]*cadvisor.TimedStore
	maxAge time.Duration
}

// NewPromStoreManager new a prometheus store manager
func NewPromStoreManager(cacheTTL time.Duration, config *types.MetricsPrometheus) PromStore {
	if config.DisableShow {
		klog.Warning("outer prometheus show is disabled")
	}

	return &promStoreManager{
		MetricsPrometheus: config,
		maxAge:            cacheTTL,
		caches:            make(map[string]*cadvisor.TimedStore),
	}
}

// Name show module name
func (p *promStoreManager) Name() string {
	return promResourceName
}

// Run entry main loop, collecting outer prometheus metrics and saving into cache.
// These metrics data could be analyzed by other module, such as health check
func (p *promStoreManager) Run(stop <-chan struct{}) {
	if len(p.Items) == 0 {
		klog.V(2).Info("outer prometheus metrics collecting is empty, just ignore")
		return
	}

	collect := func() {
		now := time.Now()
		metricsFamilies, err := p.fetchAllPromDirectMetrics()
		if err != nil {
			klog.Errorf("fetch All prometheus metrics err: %v", err)
			return
		}

		for mName, metrics := range metricsFamilies {
			mType := metrics.GetType()
			if mType != dto.MetricType_COUNTER && mType != dto.MetricType_GAUGE {
				// just for pure integer type
				continue
			}

			if len(metrics.Metric) == 0 {
				// for metrics family with only one metric, just treating the metric name as the key
				v := p.getMetricValue(mType, metrics.Metric[0])
				p.addPromStats(now, mName, v)
			} else {
				// metrics such as:
				// kubelet_docker_operations_total{operation_type="create_container"} 6
				// kubelet_docker_operations_total{operation_type="info"} 1
				// for metrics family with multi metrics, using the metric name and label value as the key
				for _, metric := range metrics.Metric {
					key := mName
					for _, label := range metric.GetLabel() {
						key = key + "_" + label.GetValue()
					}
					v := p.getMetricValue(mType, metric)
					p.addPromStats(now, key, v)
				}
			}
		}
	}

	go wait.Until(collect, p.CollectInterval.TimeDuration(), stop)
	klog.V(2).Info("outer prometheus metrics collecting started")
}

// GetPromDirectMetrics fetch all out prometheus metrics from server directly
func (p *promStoreManager) GetPromDirectMetrics() (metricFamilies map[string]*dto.MetricFamily, err error) {
	if p.DisableShow {
		return make(map[string]*dto.MetricFamily), nil
	}
	return p.fetchAllPromDirectMetrics()
}

// GetPromRangeMetrics get metric value list in range time based on assigned key from cache storage
func (p *promStoreManager) GetPromRangeMetrics(key string, start, end time.Time, count int) ([]float64, error) {
	var cache *cadvisor.TimedStore
	var values []float64

	err := func() error {
		var ok bool
		p.lock.RLock()
		defer p.lock.RUnlock()

		cache, ok = p.caches[key]
		if !ok {
			return fmt.Errorf("resource not found: %s", key)
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	sts := cache.InTimeRange(start, end, count)
	for _, s := range sts {
		values = append(values, s.(float64))
	}
	return values, nil
}

// GetPromRecentMetrics get metric newest value based on key
func (p *promStoreManager) GetPromRecentMetrics(key string) (float64, error) {
	var cache *cadvisor.TimedStore

	err := func() error {
		var ok bool
		p.lock.RLock()
		defer p.lock.RUnlock()

		cache, ok = p.caches[key]
		if !ok {
			return fmt.Errorf("resource not found: %s", key)
		}
		return nil
	}()
	if err != nil {
		return 0, err
	}

	return cache.Get(0).(float64), nil
}

// metric family, such as:
// # HELP kubelet_docker_operations_total [ALPHA] Cumulative number of Docker operations by operation type.
// # TYPE kubelet_docker_operations_total counter
// kubelet_docker_operations_total{operation_type="create_container"} 6
// kubelet_docker_operations_total{operation_type="info"} 1
func (p *promStoreManager) fetchAllPromDirectMetrics() (metricFamilies map[string]*dto.MetricFamily, err error) {
	metricFamilies = make(map[string]*dto.MetricFamily)

	for _, promData := range p.Items {
		klog.V(4).Infof("start collecting prometheus metrics for: %s", promData.Address)
		mf, err := p.fetchPromMetrics(promData)
		if err != nil {
			klog.Errorf("fetch prometheus from %s failed: %v", promData.Address, err)
			continue
		}
		for k, v := range mf {
			metricFamilies[k] = v
		}
	}

	return metricFamilies, nil
}

func (p *promStoreManager) fetchPromMetrics(prom *types.PrometheusData) (map[string]*dto.MetricFamily, error) {
	if len(prom.Address) == 0 {
		return nil, fmt.Errorf("request address is nil")
	}

	req, err := http.NewRequest("GET", prom.Address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, err
	}

	// just keep the needed metrics
	p.filterPromMetrics(metricFamilies, prom.CollectMap, prom.NoCollectMap)
	return metricFamilies, nil
}

func (p *promStoreManager) filterPromMetrics(metricFamilies map[string]*dto.MetricFamily,
	collect, noCollect sets.String) {
	if collect.Len() == 0 && noCollect.Len() == 0 {
		return
	}

	for k := range metricFamilies {
		if collect.Len() != 0 && !collect.Has(k) {
			delete(metricFamilies, k)
			continue
		}
		if noCollect.Len() != 0 && noCollect.Has(k) {
			delete(metricFamilies, k)
			continue
		}
	}

	return
}

func (p *promStoreManager) getMetricValue(mType dto.MetricType, metric *dto.Metric) float64 {
	switch mType {
	case dto.MetricType_COUNTER:
		return metric.Counter.GetValue()
	case dto.MetricType_GAUGE:
		return metric.Gauge.GetValue()
	default:
		klog.Errorf("invalid metric type: %s, return zero value", mType)
	}
	return 0
}

// addPromStats add metric value to cache
func (p *promStoreManager) addPromStats(now time.Time, name string, value float64) {
	var cache *cadvisor.TimedStore
	var ok bool

	func() {
		p.lock.Lock()
		defer p.lock.Unlock()

		cache, ok = p.caches[name]
		if !ok {
			cache = cadvisor.NewTimedStore(p.maxAge, -1)
			p.caches[name] = cache
		}
	}()

	cache.Add(now, value)
}
