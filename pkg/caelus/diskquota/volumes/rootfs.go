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
	"strings"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/runtime"
	"github.com/tencent/caelus/pkg/caelus/util/runtime/docker"

	"k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type rootFsQuotaManager struct {
	runtimeClient runtime.RuntimeClient
	offlineSize   *types.DiskQuotaSize
}

// Ensure rootfs disk quota implements volume quota interface
var _ VolumeQuotaManager = &rootFsQuotaManager{}

// NewRootFsDiskQuota news volume quota for root path
func NewRootFsDiskQuota(runtimeName string, offlineSize *types.DiskQuotaSize) VolumeQuotaManager {
	var runtimeClient runtime.RuntimeClient
	switch runtimeName {
	case types.ContainerRuntimeDocker:
		runtimeClient = dockerclient.NewDockerClient()
	default:
		klog.Fatalf("invalid container runtime: %s", runtimeName)
	}

	return &rootFsQuotaManager{
		runtimeClient: runtimeClient,
		offlineSize:   offlineSize,
	}
}

// Name returns name for root path
func (r *rootFsQuotaManager) Name() types.VolumeType {
	return types.VolumeTypeRootFs
}

// GetVolumes return root paths, which need to set quota
func (r *rootFsQuotaManager) GetVolumes(pod *v1.Pod) (map[string]*types.PathInfo, error) {
	var errs error
	paths := make(map[string]*types.PathInfo)

	size := getVolumeQuotaSize(pod, annotationKeyRootFsDiskQuotaKey, r.offlineSize)
	if size == nil || (size.Quota == 0 && size.Inodes == 0) {
		klog.V(4).Infof("roofs disk quota is nil for pod: %s-%s", pod.Namespace, pod.Name)
		return paths, nil
	}

	cids := getContainerIDs(pod)
	for _, cid := range cids {
		rootPath := r.getContainerRootPath(cid)
		if len(rootPath) == 0 {
			// return err, and check next time
			errs = fmt.Errorf("%v;root path is nil", errs)
			continue
		}
		if !util.InHostNamespace {
			rootPath = path.Join(types.RootFS, rootPath)
		}

		paths[cid[0:12]] = &types.PathInfo{
			Path: rootPath,
			Size: size,
		}
	}

	if klog.V(2).Enabled() {
		formatStr := fmt.Sprintf("\nrootFs disk quota volumes for pod(%s-%s):\n",
			pod.Namespace, pod.Name)
		for name, p := range paths {
			formatStr += fmt.Sprintf("  name: %s, path: %s, quota: %+v\n",
				name, p.Path, p.Size)
		}
		klog.Info(formatStr)
	}
	return paths, errs
}

func (r *rootFsQuotaManager) getContainerRootPath(cid string) string {
	rootPath := ""

	conJson, err := r.runtimeClient.InspectContainer(cid)
	if err != nil {
		klog.Errorf("inspect container %s err: %v", cid, err)
		return rootPath
	}

	if conJson.State.Status != "running" || !conJson.State.Running {
		klog.Errorf("container %s not running", cid)
		return rootPath
	}

	/*
			"Data": {
		      "MergedDir": "/var/lib/docker/overlay2/03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/merged",
		      "UpperDir": "/var/lib/docker/overlay2/03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/diff",
		      "WorkDir": "/var/lib/docker/overlay2/03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/work"
		    }
	*/
	if mergedPath, ok := conJson.GraphDriver.Data["MergedDir"]; ok {
		rootPath = strings.TrimSuffix(mergedPath, "/merged")
	} else {
		klog.Errorf("merged dir not found for container %s", cid)
	}

	return rootPath
}

func getContainerIDs(pod *v1.Pod) []string {
	var cids []string

	for _, cs := range pod.Status.ContainerStatuses {
		cid := strings.TrimPrefix(cs.ContainerID, "docker://")
		// cid may be empty when pod is in creating status
		if len(cid) != 0 {
			cids = append(cids, cid)
		} else {
			klog.Warningf("no container id found, pod(%s-%s) may be in creating status",
				pod.Namespace, pod.Name)
		}
	}

	return cids
}
