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

package types

import (
	"github.com/tencent/caelus/pkg/util/times"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

const (
	// container runtime
	ContainerRuntimeDocker = "docker"
)

var (
	defaultContainerRuntime   = ContainerRuntimeDocker
	availableContainerRuntime = sets.NewString(ContainerRuntimeDocker)

	VolumeTypeRootFs     VolumeType = "rootFs"
	VolumeTypeEmptyDir   VolumeType = "emptyDir"
	VolumeTypeHostPath   VolumeType = "hostPath"
	AvailableVolumeTypes            = sets.NewString(
		VolumeTypeRootFs.String(),
		VolumeTypeEmptyDir.String(),
		VolumeTypeHostPath.String())
)

type VolumeType string

// String output volume type to string
func (vt VolumeType) String() string {
	return string(vt)
}

// DiskQuotaConfig group disk quota configurations
type DiskQuotaConfig struct {
	Enabled     bool           `json:"enabled"`
	CheckPeriod times.Duration `json:"check_period"`
	// such as docker or containerd
	ContainerRuntime string `json:"container_runtime"`
	// quota size just for offline job, online jobs need to announce in annotations
	VolumeSizes map[VolumeType]*DiskQuotaSize `json:"volume_sizes"`
}

// shall we support soft feature ?
type DiskQuotaSize struct {
	Quota      uint64 `json:"quota"`
	Inodes     uint64 `json:"inodes"`
	QuotaUsed  uint64 `json:"-"`
	InodesUsed uint64 `json:"-"`
}

func initDiskQuotaManager(config *DiskQuotaConfig) {
	if config.CheckPeriod.Seconds() == 0 {
		config.CheckPeriod = defaultCheckPeriod
	}
	if len(config.ContainerRuntime) == 0 {
		config.ContainerRuntime = defaultContainerRuntime
	}
	if !availableContainerRuntime.Has(config.ContainerRuntime) {
		klog.Fatalf("invalid container runtime %s, should be one of the: %v",
			config.ContainerRuntime, availableContainerRuntime)
	}
}

// SharedInfo indicate a path has shared quota or not
type SharedInfo struct {
	PodName string
}

// PathInfo group path and quota options
type PathInfo struct {
	Path string
	Size *DiskQuotaSize
	//if we set share limit, SharedInfo containers project id name
	//if not, SharedInfo is nil
	SharedInfo *SharedInfo
}
