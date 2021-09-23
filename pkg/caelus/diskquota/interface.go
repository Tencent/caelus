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

package diskquota

import (
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"k8s.io/api/core/v1"
)

// DiskQuotaInterface describe disk quota functions
type DiskQuotaInterface interface {
	// module name
	Name() string
	// Run main loop
	Run(stop <-chan struct{})
	// GetPodDiskQuota return all volumes of the pod
	GetPodDiskQuota(pod *v1.Pod) (map[types.VolumeType]*VolumeInfo, error)
	// GetAllPodsDiskQuota return all volumes for all the pods on the node
	GetAllPodsDiskQuota() ([]*PodVolumes, error)
}

// PathInfoWrapper describe options for path info
type PathInfoWrapper struct {
	// if set quota successfully
	setQuotaSuccess bool
	types.PathInfo
}

// VolumeInfo describes volume path and quota size
type VolumeInfo struct {
	getPathsSuccess  bool
	setQuotasSuccess bool
	// volume name => path info
	Paths map[string]*PathInfoWrapper
}

// PodVolumes describes volume info, such as path and quota
type PodVolumes struct {
	Pod      *v1.Pod
	AppClass appclass.AppClass
	Volumes  map[types.VolumeType]*VolumeInfo
	// record the set quota timestamp, to check if the pod has exited
	lastTime time.Time
}
