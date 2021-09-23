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

package cpi

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/alarm"

	"github.com/tencent/caelus/pkg/caelus/types"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

var (
	defaultManager *CPIManager
)

// CPIManager compares aggregated CPI data from prometheus and local CPI data to detect outliers and antagonists
type CPIManager struct {
	sync.RWMutex
	// window determines how much data is analyzed to find antagonists when outliers are detected
	window time.Duration
	// jobSpecQueryRange is the time range used to query history jobSpec from prometheus
	jobSpecQueryRange time.Duration
	// client is the client to query prometheus
	client api.Client
	// cpiCache caches local CPI data
	cpiCache map[TaskMeta]*timeSeries
	// cpuCache caches local CPU usage data
	cpuCache map[TaskMeta]*timeSeries
	// jobSpecCache caches latest jobSpec received from prometheus
	jobSpecCache map[string]JobSpec
}

// InitManager initializes default CPIManager
func InitManager(cfg types.CPIManagerConfig) error {
	if defaultManager != nil {
		return nil
	}

	c, err := api.NewClient(api.Config{
		Address: cfg.PrometheusAddr.String(),
	})
	if err != nil {
		return fmt.Errorf("init prometheus client failed: %v", err)
	}
	defaultManager = &CPIManager{
		window:            cfg.WindowDuration.TimeDuration(),
		jobSpecQueryRange: cfg.MaxJobSpecRange.TimeDuration(),
		client:            c,
		cpiCache:          make(map[TaskMeta]*timeSeries),
		cpuCache:          make(map[TaskMeta]*timeSeries),
		jobSpecCache:      make(map[string]JobSpec),
	}
	go wait.Forever(defaultManager.identifyAntagonists, 5*time.Minute)
	go wait.Forever(defaultManager.deleteExpiredCache, 5*time.Minute)
	klog.Infof("started cpi manager")
	return nil
}

// AddRecord adds record to default CPIManager
func AddRecord(record CPIRecord) {
	if defaultManager != nil {
		defaultManager.AddRecord(record)
	}
}

// AddRecord caches record in CPIManager
func (m *CPIManager) AddRecord(record CPIRecord) {
	m.Lock()
	defer m.Unlock()
	t := TaskMeta{
		JobName:  record.JobName,
		TaskName: record.TaskName,
	}
	cpiSeries, ok := m.cpiCache[t]
	if !ok {
		cpiSeries = newTimeSeries()
		m.cpiCache[t] = cpiSeries
	}
	cpiSeries.add(record.CPI, record.Timestamp)

	cpuSeries, ok := m.cpuCache[t]
	if !ok {
		cpuSeries = newTimeSeries()
		m.cpuCache[t] = cpuSeries
	}
	cpuSeries.add(record.CPUUsage, record.Timestamp)
}

// identifyAntagonists detects outliers and try to find antagonists
func (m *CPIManager) identifyAntagonists() {
	m.RLock()
	defer m.RUnlock()
	// update job spec
	for task := range m.cpiCache {
		if spec, ok := m.jobSpecCache[task.JobName]; !ok {
			klog.Infof("job %s has no local job spec, try query from prometheus", task.JobName)
		} else if spec.ExpiredAt.Before(time.Now()) {
			klog.Infof("job spec for %s expired, try query from prometheus", task.JobName)
		} else {
			continue
		}
		meanMatrix, err := query(m.client,
			fmt.Sprintf("cpi_spec_mean{job_name=%q}[%s]", task.JobName, truncDuration(m.jobSpecQueryRange)))
		if err != nil {
			klog.Errorf("query cpi_spec_mean for job %s failed: %v", task.JobName, err)
			continue
		} else if len(meanMatrix) == 0 {
			klog.Infof("retrieved 0 cpi_spec_mean sample for %s, jobSpec update skipped", task.JobName)
			continue
		}

		stddevMatrix, err := query(m.client,
			fmt.Sprintf("cpi_spec_stddev{job_name=%q}[%s]", task.JobName, truncDuration(m.jobSpecQueryRange)))
		if err != nil {
			klog.Errorf("query cpi_spec_stddev for job %s failed: %v", task.JobName, err)
			continue
		} else if len(stddevMatrix) == 0 {
			klog.Infof("retrieved 0 cpi_spec_stddev sample for %s, jobSpec update skipped", task.JobName)
			continue
		}

		jobSpec, err := makeJobSpec(meanMatrix, stddevMatrix)
		if err != nil {
			klog.Errorf("merge jobSpec for job %s failed: %v", task.JobName, err)
		}
		if jobSpec != nil {
			klog.Infof("update local jobSpec: %s", jobSpec)
			m.jobSpecCache[task.JobName] = *jobSpec
		}
	}

	outliers := findOutlier(m.cpiCache, m.cpuCache, m.jobSpecCache)
	if len(outliers) == 0 {
		return
	}

	// find antagonists
	antagonists := make(map[TaskMeta][]Antagonist)
	now := time.Now()
	for _, o := range outliers {
		threshold := m.jobSpecCache[o.JobName].outlierThreshold()
		windowStart := now.Add(-1 * m.window)
		victimCPI := m.cpiCache[o].rangeSearch(windowStart, now)
		for suspect, values := range m.cpuCache {
			var correlation float64
			suspectCPU := values.rangeSearch(windowStart, now).normalize()
			for _, t := range victimCPI.timeline {
				u, ok := suspectCPU.getByUnixNano(t)
				if !ok {
					u = 0
				}
				c, _ := victimCPI.getByUnixNano(t)
				if c > threshold {
					correlation += u * (1 - threshold/c)
				} else {
					correlation += u * (c/threshold - 1)
				}
			}
			antagonists[o] = append(antagonists[o], Antagonist{
				TaskMeta:    suspect,
				correlation: correlation,
			})
		}
	}
	for victim, anta := range antagonists {
		for _, a := range anta {
			msg := fmt.Sprintf("victim %s, antagonist %s, correlation: %f", victim, a.TaskMeta, a.correlation)
			klog.Infof(msg)
			alarm.SendAlarm(msg)
		}
	}
}

// deleteExpiredCache deletes expired data in caches
func (m *CPIManager) deleteExpiredCache() {
	m.Lock()
	defer m.Unlock()
	deadline := time.Now().Add(-2 * m.window)
	jobSet := make(map[string]struct{})
	for t, cpiSeries := range m.cpiCache {
		cpiSeries.expire(deadline)
		if len(m.cpiCache[t].data) == 0 {
			delete(m.cpiCache, t)
		} else {
			jobSet[t.JobName] = struct{}{}
		}
	}

	for t, cpuSeries := range m.cpuCache {
		cpuSeries.expire(deadline)
		if len(m.cpuCache[t].data) == 0 {
			delete(m.cpuCache, t)
		}
	}

	// delete job spec only if we don't have any cpi cache for that job.
	// expired job specs are not deleted, because expired job spec is better than no job spec
	for jobName := range m.jobSpecCache {
		if _, ok := jobSet[jobName]; !ok {
			delete(m.jobSpecCache, jobName)
		}
	}
}

// findOutlier finds outliers by comparing cpi data of each task to jobSpec
func findOutlier(cpiCache map[TaskMeta]*timeSeries, cpuCache map[TaskMeta]*timeSeries,
	jobSpecCache map[string]JobSpec) []TaskMeta {
	now := time.Now()
	var outlier []TaskMeta
	for meta, ts := range cpiCache {
		jobSpec, ok := jobSpecCache[meta.JobName]
		if !ok {
			klog.Warningf("task %s has no local job spec, will not judge outlier", meta.JobName)
			continue
		} else if jobSpec.ExpiredAt.Before(time.Now()) {
			klog.Warningf("task %s is using expired jobSpec", meta.JobName)
		}
		threshold := jobSpec.outlierThreshold()
		cpiValues := ts.rangeSearch(now.Add(-5*time.Minute), now)
		abnormalCnt := 0
		for t, cpi := range cpiValues.data {
			cpuUsage, ok := cpuCache[meta].getByUnixNano(t)
			if !ok {
				klog.Infof("can't find cpu usage of %s at %s", meta, time.Unix(0, t).Format(time.RFC3339))
				continue
			} else if cpuUsage < 0.25 {
				klog.Infof("skip cpi of %s, because cpu usage is %.3f", meta, cpuUsage)
				continue
			}

			if cpi >= threshold {
				abnormalCnt++
				klog.Infof("%s above threshold. cpi: %.3f, threshold: %.3f", meta, cpi, threshold)
			}
		}
		if abnormalCnt >= 3 {
			outlier = append(outlier, meta)
			klog.Infof("detected outlier %s", meta)
		}
	}
	return outlier
}

// query queries prometheus using query statement
func query(client api.Client, query string) (model.Matrix, error) {
	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := v1api.Query(ctx, query, time.Time{})
	if err != nil {
		klog.Infof("Error querying Prometheus: %v\n", err)
		return nil, err
	}
	if len(warnings) > 0 {
		klog.Infof("Warnings: %v\n", warnings)
	}
	m, ok := result.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("result of %s not matrix", query)
	}
	return m, nil
}

// getLatestSample iterates all samples in matrix to find latest jobSpec for each job
func getLatestSample(m model.Matrix) (*sampleWithTTL, error) {
	var min *sampleWithTTL
	for _, s := range m {
		ttl, err := time.ParseDuration(string(s.Metric["ttl"]))
		if err != nil {
			return nil, fmt.Errorf("parse ttl failed, raw %s, err: %v", s.Metric["ttl"], err)
		}
		latestSample := s.Values[len(s.Values)-1]
		createdAt := latestSample.Timestamp.Time()
		if min == nil || createdAt.After(min.sample.Timestamp.Time()) {
			min = &sampleWithTTL{
				metric:    s.Metric,
				sample:    latestSample,
				expiredAt: createdAt.Add(ttl),
			}
		}
	}
	return min, nil
}

// makeJobSpec makes jobSpec by merging mean and stddev
func makeJobSpec(meanMatrix, stddevMatrix model.Matrix) (*JobSpec, error) {
	meanSample, err := getLatestSample(meanMatrix)
	if err != nil {
		return nil, fmt.Errorf("get min ttl mean matrix failed: %v", err)
	} else if meanSample == nil {
		return nil, nil
	}
	stddevSample, err := getLatestSample(stddevMatrix)
	if err != nil {
		return nil, fmt.Errorf("get min ttl stddev matrix failed: %v", err)
	} else if stddevSample == nil {
		return nil, nil
	}

	if meanSample.sample.Timestamp != stddevSample.sample.Timestamp {
		klog.Warningf("mean sample created at %s, but steddev sample created at %s, use existing local job spec",
			meanSample.sample.Timestamp, stddevSample.sample.Timestamp)
		return nil, nil
	}
	jobName := meanSample.metric["job_name"]
	mean, err := strconv.ParseFloat(meanSample.sample.Value.String(), 64)
	if err != nil {
		return nil, fmt.Errorf("parse cpi mean for job %s failed, raw %s, err: %v",
			jobName, meanSample.sample.Value.String(), err)
	}
	stddev, err := strconv.ParseFloat(stddevSample.sample.Value.String(), 64)
	if err != nil {
		return nil, fmt.Errorf("parse cpi stddev for job %s failed, raw %s, err: %v",
			jobName, stddevSample.sample.Value.String(), err)
	}
	ret := &JobSpec{
		JobName:   string(jobName),
		CPIMean:   mean,
		CPIStdDev: stddev,
		ExpiredAt: meanSample.expiredAt,
	}
	return ret, nil
}

// truncDuration truncates 0 values from dur.
// example:
// before truncate: 10h0m0s
// after truncate: 10h
func truncDuration(dur time.Duration) string {
	reg := regexp.MustCompile(`[0-9]*[a-zA-Z]`)
	return string(reg.Find([]byte(dur.String())))
}

// sampleWithTTL wraps metric with its latest sample and expire time
type sampleWithTTL struct {
	metric    model.Metric
	sample    model.SamplePair
	expiredAt time.Time
}

// CPIRecord contains cpi and cpu data for a task
type CPIRecord struct {
	TaskMeta
	CPI       float64
	CPUUsage  float64
	Timestamp time.Time
}

// JobSpec contains aggregated cpi data for a job
type JobSpec struct {
	JobName   string
	CPIMean   float64
	CPIStdDev float64
	ExpiredAt time.Time
}

// outlierThreshold calculates outlier threshold based on cpi mean and cpi std_dev
func (s JobSpec) outlierThreshold() float64 {
	return s.CPIMean + 2*s.CPIStdDev
}

// String prints JobSpec
func (s JobSpec) String() string {
	return fmt.Sprintf("{jobName: %s, mean: %.3f, stdDev: %.3f, expiredAt: %s}",
		s.JobName, s.CPIMean, s.CPIStdDev, s.ExpiredAt.Format(time.RFC3339))
}

// TaskMeta describes tasks
type TaskMeta struct {
	JobName  string
	TaskName string
}

// String prints TaskMeta
func (t TaskMeta) String() string {
	return fmt.Sprintf("{job: %s, task: %s}", t.JobName, t.TaskName)
}

// Antagonist describe antagonist tasks information
type Antagonist struct {
	TaskMeta
	correlation float64
}
