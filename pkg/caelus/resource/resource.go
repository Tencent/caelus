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

package resource

import (
	"fmt"
	"time"

	"github.com/tencent/caelus/pkg/caelus/checkpoint"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/predict"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/util/times"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

var _ Interface = (*offlineOnK8sManager)(nil)

// offlineOnK8sManager describe node resource manager for offline jobs on k8s, including yarn on k8s
type offlineOnK8sManager struct {
	client clientInterface

	updateInterval time.Duration
	predictor      predict.Interface
	conflict       conflict.Manager
	// eventChan is the channel for receiving node resource update event
	eventChan chan *types.ResourceUpdateEvent

	// silence state used to support running offline jobs in some periods
	scheduleSilence         bool
	aheadOfUnschedulePeriod time.Duration
	silencePeriods          [][2]times.SecondsInDay
}

// OfflineOnK8sCommonData describe common data for offline jobs on k8s, including yarn on k8s
type OfflineOnK8sCommonData struct {
	// container state
	StStore statestore.StateStore
	// k8s client
	Client kubernetes.Interface
	// pod informer
	PodInformer cache.SharedIndexInformer
	// checkpointManager
	CheckpointManager *checkpoint.NodeResourceCheckpointManager
}

// NewOfflineOnK8sManager new an instance for node resource manager
func NewOfflineOnK8sManager(config types.NodeResourceConfig, predictor predict.Interface,
	conflict conflict.Manager, offlineData interface{}) Interface {
	manager := &offlineOnK8sManager{
		updateInterval:          config.UpdateInterval.TimeDuration(),
		predictor:               predictor,
		conflict:                conflict,
		eventChan:               make(chan *types.ResourceUpdateEvent, 32),
		silencePeriods:          config.Silence.Periods,
		aheadOfUnschedulePeriod: config.Silence.AheadOfUnSchedule.TimeDuration(),
	}
	klog.Infof("flag for disable killing pod when no conflict: %v", config.DisableKillIfNormal)
	switch config.OfflineType {
	case types.OfflineTypeOnk8s:
		manager.client = newK8sClient(config, predictor, conflict, offlineData)
	case types.OfflineTypeYarnOnk8s:
		manager.client = newYarnClient(config, predictor, conflict, offlineData)
	default:
		klog.Fatalf("not supported: %s", config.OfflineType)
	}

	return manager
}

// Name module name
func (m *offlineOnK8sManager) Name() string {
	return "ModuleResourceOnK8s"
}

// Run main loop
func (m *offlineOnK8sManager) Run(stopCh <-chan struct{}) {
	go func() {
		// init function may be waiting to be ready
		err := m.client.Init()
		if err != nil {
			klog.Errorf("node resource manager init err: %v", err)
		}

		// check schedule state
		err = m.client.CheckPoint()
		if err != nil {
			klog.Errorf("node resource manager checkpoint err: %v", err)
		}

		// start client thread
		m.client.Run(stopCh)

		// main loop
		ticker := time.NewTicker(m.updateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				klog.V(4).Infof("update node resource periodically")
				m.checkSilence()
				m.sync(nil)
			case event := <-m.eventChan:
				klog.Infof("update node resource for receiving event: %+v", event)
				m.sync(event.ConflictRes)
				klog.Infof("update node resource done")
			}
		}
	}()
}

func (m *offlineOnK8sManager) checkSilence() {
	if len(m.silencePeriods) == 0 {
		return
	}

	now := time.Now()
	unScheduleTime := now.Add(m.aheadOfUnschedulePeriod)
	scheduleSilence := false
	resourceSilence := false

	for _, ss := range m.silencePeriods {
		if times.IsTimeInSecondsDay(unScheduleTime, ss) {
			scheduleSilence = true
			break
		}
	}
	if !m.scheduleSilence && scheduleSilence {
		klog.V(2).Infof("starting silence schedule to disabled state")
		m.client.DisableOfflineSchedule()
		m.scheduleSilence = true
	}

	for _, ss := range m.silencePeriods {
		if times.IsTimeInSecondsDay(now, ss) {
			resourceSilence = true
			break
		}
	}
	if util.SilenceMode != resourceSilence {
		// assign new silence mode
		util.SilenceMode = resourceSilence

		if resourceSilence {
			klog.V(2).Infof("staring silence offline resource")
		} else {
			klog.V(2).Infof("starting un-silence schedule to enabled state")
			m.scheduleSilence = false
			m.client.EnableOfflineSchedule()
			klog.V(2).Infof("staring un-silence offline resource")
		}
	}

	return
}

// DisableOfflineSchedule disable schedule
func (m *offlineOnK8sManager) DisableOfflineSchedule() error {
	if m.scheduleSilence {
		klog.Infof("Silence mode, ignore schedule disable")
		return nil
	}
	return m.client.DisableOfflineSchedule()
}

// EnableOfflineSchedule enable schedule
func (m *offlineOnK8sManager) EnableOfflineSchedule() error {
	if m.scheduleSilence {
		klog.V(4).Infof("Silence mode, ignore schedule enable")
		return nil
	}
	return m.client.EnableOfflineSchedule()
}

// OfflineScheduleDisabled return true if schedule disabled for offline jobs
func (m *offlineOnK8sManager) OfflineScheduleDisabled() bool {
	return m.client.OfflineScheduleDisabled()
}

// SyncNodeResource receive event to update offline resource capacity timely
func (m *offlineOnK8sManager) SyncNodeResource(event *types.ResourceUpdateEvent) error {
	select {
	case m.eventChan <- event:
	default:
		klog.Errorf("resource manager event chan is full, dropping event: %+v", event)
		return fmt.Errorf("node update channel is full")
	}

	return nil
}

// GetOfflineJobs returns current running offline job list
func (m *offlineOnK8sManager) GetOfflineJobs() ([]types.OfflineJobs, error) {
	return m.client.GetOfflineJobs()
}

// KillOfflineJob kill offline jobs depending on conflicting resource
func (m *offlineOnK8sManager) KillOfflineJob(conflictingResource v1.ResourceName) {
	klog.V(2).Infof("start killing offline job based on conflicting resource %s", conflictingResource)
	m.client.KillOfflineJob(conflictingResource)
}

// Describe implement prometheus interface
func (m *offlineOnK8sManager) Describe(ch chan<- *prometheus.Desc) {
	m.client.Describe(ch)
}

// Collect implement prometheus interface
func (m *offlineOnK8sManager) Collect(ch chan<- prometheus.Metric) {
	m.client.Collect(ch)
}

func (m *offlineOnK8sManager) sync(conflictingResources []string) {
	// get predict resources
	resList := m.predictor.GetAllocatableForBatch()
	if resList == nil {
		klog.V(2).Infof("predict resource is nil")
		return
	}
	klog.V(4).Infof("sync node resources, predict resources: %+v", resList)

	// check conflicting resources
	conflictedList, _ := m.conflict.CheckAndSubConflictResource(resList)
	if len(conflictedList) > 0 {
		klog.Infof("finding conflict resources(%v), offline resources changed to: %+v",
			conflictedList, resList)
	}
	klog.V(4).Infof("sync node resources, after remove conflicting: %+v", resList)
	metrics.NodeResourceMetricsReset(resList, metrics.NodeResourceTypeOfflineConflict)

	// if entry silence mode, just set cpu as zero
	if util.SilenceMode {
		klog.V(4).Infof("update node resource entry silence mode, just set cpu as zero")
		resList[v1.ResourceCPU] = resource.MustParse("0")
		conflictingResources = []string{string(v1.ResourceCPU)}
	}

	// adapt and update the predict resources based on some conditions
	m.client.AdaptAndUpdateOfflineResource(resList, conflictingResources)
}

// conflictingResource return the first conflicting resource name, and empty if no conflicting resource found
func conflictingResource(conflictingList map[v1.ResourceName]bool) v1.ResourceName {
	for res, conflicting := range conflictingList {
		if !conflicting {
			continue
		}
		// return first resource if there are multi resource types
		return res
	}

	return ""
}
