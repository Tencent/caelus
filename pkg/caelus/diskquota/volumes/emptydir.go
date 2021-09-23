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

package volume

import (
	"fmt"
	"path"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"k8s.io/api/core/v1"
	"k8s.io/klog"
)

type emptyDirQuotaManager struct {
	offlineSize *types.DiskQuotaSize
	// emptyDir will be created under kubelet root directory automatically
	kubeletRootDir string
}

// Ensure empty dir disk quota implements volume quota interface
var _ VolumeQuotaManager = &emptyDirQuotaManager{}

// nilVolumeQuotaSize check size is nil or not
func nilVolumeQuotaSize(size *types.DiskQuotaSize) bool {
	return (size == nil || (size.Quota == 0 && size.Inodes == 0))
}

// NewEmptyDirQuota create empty dir disk quota manager
func NewEmptyDirQuota(offlineSize *types.DiskQuotaSize, kubeletRootDir string) VolumeQuotaManager {
	return &emptyDirQuotaManager{
		offlineSize:    offlineSize,
		kubeletRootDir: kubeletRootDir,
	}
}

// Name returns name for empty dir
func (e *emptyDirQuotaManager) Name() types.VolumeType {
	return types.VolumeTypeEmptyDir
}

// GetVolumes return empty dir paths, which need to set quota
func (e *emptyDirQuotaManager) GetVolumes(pod *v1.Pod) (map[string]*types.PathInfo, error) {
	paths := make(map[string]*types.PathInfo)

	size := getVolumeQuotaSize(pod, annotationKeyEmptyDirDiskQuotaKey, e.offlineSize)
	if nilVolumeQuotaSize(size) {
		klog.V(2).Infof("exclusive empty dir disk quota is nil for pod(%s-%s),"+
			"checking if volume has set limit", pod.Namespace, pod.Name)
	}
	// totalSize means all empty directories share the same limit size with same project id
	totalSize := getVolumeQuotaSize(pod, annotationKeyTotalEmptyDirDiskQuotaKey, e.offlineSize)
	if nilVolumeQuotaSize(totalSize) {
		klog.V(2).Infof("shared empty dir disk quota is nil for pod(%s-%s),"+
			"checking if volume has set limit", pod.Namespace, pod.Name)
	} else if !nilVolumeQuotaSize(size) {
		klog.V(2).Infof("both shared and exclusive quota for pod(%s-%s) is set, ignore shared quota.",
			pod.Namespace, pod.Name)
	}

	for _, v := range pod.Spec.Volumes {
		if v.EmptyDir == nil {
			continue
		}

		// diskquota is not available for memory volume
		if v.EmptyDir.Medium == v1.StorageMediumMemory {
			klog.V(3).Infof("StorageMediumMemory not supported for pod %s-%s with empty dir: %s",
				pod.Namespace, pod.Name, v.Name)
			continue
		}

		var newSize *types.DiskQuotaSize
		var sharedInfo *types.SharedInfo
		// if empty dir limit size is set, using the value.
		if v.EmptyDir.SizeLimit != nil {
			klog.V(3).Infof("empty dir limit size found for pod %s-%s with name: %s",
				pod.Namespace, pod.Name, v.Name)
			newSize = &types.DiskQuotaSize{
				Quota: uint64(v.EmptyDir.SizeLimit.Value()),
			}
			if size != nil {
				newSize.Inodes = size.Inodes
			}
		} else {
			newSize = size
		}

		if nilVolumeQuotaSize(newSize) {
			if nilVolumeQuotaSize(totalSize) {
				continue
			}
			newSize = totalSize
			sharedInfo = &types.SharedInfo{
				PodName: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
			}

		}

		emptyDirPath := path.Join(e.kubeletRootDir, "pods", string(pod.UID), "volumes",
			"kubernetes.io~empty-dir", v.Name)
		if !util.InHostNamespace {
			emptyDirPath = path.Join(types.RootFS, emptyDirPath)
		}

		paths[v.Name] = &types.PathInfo{
			Path:       emptyDirPath,
			Size:       newSize,
			SharedInfo: sharedInfo,
		}
	}

	if klog.V(2) {
		formatStr := fmt.Sprintf("\nemptyDir disk quota volumes for pod(%s-%s):\n",
			pod.Namespace, pod.Name)
		for name, p := range paths {
			formatStr += fmt.Sprintf("  name: %s, path: %s, quota: %+v ,shared: %v\n",
				name, p.Path, p.Size, p.SharedInfo != nil)
		}
		klog.Info(formatStr)
	}
	return paths, nil
}
