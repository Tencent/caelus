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
	"github.com/tencent/caelus/pkg/caelus/types"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/api/core/v1"
)

// commonResourceInterface describe common functions for resource manager
type commonResourceInterface interface {
	// module name
	Name() string
	// Run main loop
	Run(stopCh <-chan struct{})
	// DisableOfflineSchedule disable schedule
	DisableOfflineSchedule() error
	// EnableOfflineSchedule enable schedule
	EnableOfflineSchedule() error
	// OfflineScheduleDisabled return true if schedule disabled for offline jobs
	OfflineScheduleDisabled() bool
	// GetOfflineJobs return current offline job list
	GetOfflineJobs() ([]types.OfflineJobs, error)
	// KillOfflineJob kill offline job based on conflicting resource
	KillOfflineJob(conflictingResource v1.ResourceName)

	prometheus.Collector
}

// Interface is the manager used to update offline resource capacity
type Interface interface {
	commonResourceInterface
	// SyncNodeResource receive event to update offline resource capacity timely
	SyncNodeResource(event *types.ResourceUpdateEvent) error
}

// clientInterface describe how to update offline resource capacity
type clientInterface interface {
	// common functions, using embedding interface
	commonResourceInterface
	// Init do some initializations
	Init() error
	// CheckPoint recover scheduler state from backup
	CheckPoint() error
	// AdaptAndUpdateOfflineResource adapt and update the resource list based on some conditions
	AdaptAndUpdateOfflineResource(offlineList v1.ResourceList, conflictingResources []string) error
}
