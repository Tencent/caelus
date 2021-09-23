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
	"strings"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

const (
	annotationKeyRootFsDiskQuotaKey        = types.PodAnnotationPrefix + "rootfs-diskquota"
	annotationKeyEmptyDirDiskQuotaKey      = types.PodAnnotationPrefix + "emptydir-diskquota"
	annotationKeyTotalEmptyDirDiskQuotaKey = types.PodAnnotationPrefix + "emptydir-diskquota-total"
	annotationKeyHostPathDiskQuotaKey      = types.PodAnnotationPrefix + "hostpath-diskquota"
)

// VolumeQuota describes functions for volume quota
type VolumeQuotaManager interface {
	// Name return volume name
	Name() types.VolumeType
	// GetVolumes return paths, which need to set quota
	GetVolumes(pod *v1.Pod) (map[string]*types.PathInfo, error)
}

// getVolumeQuotaSize return quota in annotations
func getVolumeQuotaSize(pod *v1.Pod, quotaKey string, offlineSize *types.DiskQuotaSize) *types.DiskQuotaSize {
	quotaSize := convertDiskQuotaFromAnnotations(pod.Annotations, quotaKey)
	// if the job is offline and no quota announced, using default quota from config
	if appclass.IsOffline(pod) && quotaSize == nil {
		return offlineSize
	}

	return quotaSize
}

// quota in annotations, such as, mixer.kubernetes.io/hostpath-diskquota: quota=10240000000,inodes=10000
func convertDiskQuotaFromAnnotations(annos map[string]string, key string) *types.DiskQuotaSize {
	var quotaSize *types.DiskQuotaSize

	valueStr, ok := annos[key]
	if !ok {
		return quotaSize
	}

	quotaExpressions := strings.Split(valueStr, ",")
	for _, exp := range quotaExpressions {
		ranges := strings.Split(exp, "=")
		if len(ranges) != 2 {
			klog.Errorf("invalid disk quota expression from annotation %s, just ignore: %s", key, exp)
			continue
		}

		quotaName := strings.TrimSpace(ranges[0])
		quantity, err := resource.ParseQuantity(strings.TrimSpace(ranges[1]))
		if err != nil {
			klog.Errorf("convert disk quota %s from annotation(%s) failed: %v", exp, key, err)
			continue
		}

		if quantity.Value() < 0 {
			klog.Errorf("convert disk quota %s from annotation(%s) failed: wrong values: %d", exp, key, quantity.Value())
			continue
		}

		if quotaSize == nil {
			quotaSize = &types.DiskQuotaSize{}
		}
		switch quotaName {
		case "quota":
			quotaSize.Quota = uint64(quantity.Value())
		case "inodes":
			quotaSize.Inodes = uint64(quantity.Value())
		default:
			klog.Errorf("invalid disk quota key %s from annotation(%s)", quotaName, exp)
		}
	}

	return quotaSize
}
