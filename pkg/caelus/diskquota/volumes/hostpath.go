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
	"os"
	"path"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/mountpoint"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	pathSeparator = "/"
)

var (
	// paths which no need to set disk quota
	rootfsPaths = sets.NewString("/bin", "/boot", "/dev", "/etc",
		"/home", "/lib", "/lib64", "/media", "/mnt", "/opt", "/proc",
		"/root", "/run", "/sbin", "/srv", "/sys", "/tmp", "/usr", "/var")
)

type hostPathQuotaManager struct {
	offlineSize *types.DiskQuotaSize
}

// Ensure host path disk quota implements volume quota interface
var _ VolumeQuotaManager = &hostPathQuotaManager{}

// NewHostPathQuota news host path volume manager
func NewHostPathQuota(offlineSize *types.DiskQuotaSize) VolumeQuotaManager {
	return &hostPathQuotaManager{
		offlineSize: offlineSize,
	}
}

// Name return module name
func (h *hostPathQuotaManager) Name() types.VolumeType {
	return types.VolumeTypeHostPath
}

// GetVolumes return host paths, which need to set quota
func (h *hostPathQuotaManager) GetVolumes(pod *v1.Pod) (map[string]*types.PathInfo, error) {
	var errs error
	paths := make(map[string]*types.PathInfo)

	size := getVolumeQuotaSize(pod, annotationKeyHostPathDiskQuotaKey, h.offlineSize)
	if size == nil || (size.Quota == 0 && size.Inodes == 0) {
		klog.V(4).Infof("host path disk quota is nil for pod: %s-%s", pod.Namespace, pod.Name)
		return paths, nil
	}

	for _, v := range pod.Spec.Volumes {
		if v.HostPath == nil {
			continue
		}

		targetPath := v.HostPath.Path
		if !util.InHostNamespace {
			targetPath = path.Join(types.RootFS, targetPath)
		}
		// check if host path is normal
		f, err := os.Stat(targetPath)
		if err != nil {
			klog.Errorf("host path(%s) for pod(%s-%s) stat err: %v",
				targetPath, pod.Namespace, pod.Name, err)
			errs = fmt.Errorf("%v;%v", errs, err)
			continue
		}
		if !f.IsDir() {
			continue
		}

		if ok, err := checkHostPathAvailable(targetPath); !ok {
			klog.Errorf("host path(%s) for pod(%s-%s) is not available for setting disk quota: %v",
				v.Name, pod.Namespace, pod.Name, err)
			continue
		}

		paths[v.Name] = &types.PathInfo{
			Path: targetPath,
			Size: size,
		}
	}

	if klog.V(2) {
		formatStr := fmt.Sprintf("\nhostPath disk quota volumes for pod(%s-%s):\n",
			pod.Namespace, pod.Name)
		for name, p := range paths {
			formatStr += fmt.Sprintf("  name: %s, path: %s, quota: %+v\n",
				name, p.Path, p.Size)
		}
		klog.Info(formatStr)
	}
	return paths, errs
}

// checkHostPathAvailable filter paths, which no need to set disk quota
func checkHostPathAvailable(targetPath string) (bool, error) {
	slices := strings.Split(targetPath, pathSeparator)
	if len(slices) > 1 {
		if rootfsPaths.Has(pathSeparator + slices[1]) {
			return false, fmt.Errorf("%s is in root fs directory", targetPath)
		}
	}

	// Do not set quota for mount point path!
	mount, err := mountpoint.FindMount(targetPath)
	if err != nil {
		return false, fmt.Errorf("get mount info for host path(%s) err: %v", targetPath, err)
	}
	if mount.Path == targetPath {
		return false, fmt.Errorf("%s is mount point, do not set quota", targetPath)
	}

	return true, nil
}
