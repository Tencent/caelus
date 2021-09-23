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

package pidlimit

import (
	"context"
	"os"
	"strconv"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/docker"

	"github.com/docker/docker/runconfig"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog"
)

/*
	Pid limit is used to limit pid number for pod with assigned label. Kubernetes could limit pid number for all pods
on the node, but not for signal pod.
*/
func init() {
	_, err := os.Stat("/sys/fs/cgroup/pids/")
	if err == nil {
		docker.PreHookCreateContainer.RegisterSubPlugin(opt.handle)
	}
}

const (
	annotationPidsLimit = plugin.PodAnnotationPrefix + "pids-limit"
)

var (
	opt = &pidsLimit{}
)

type pidsLimit struct{}

func (p *pidsLimit) handle(
	toolKits *docker.ToolKits,
	containerConfig *runconfig.ContainerConfigWrapper,
	metadata *plugin.PodMetadata) error {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(plugin.DockerPidsLimit) {
		return nil
	}

	if metadata.ContainerType != plugin.ContainerTypeLabelContainer {
		return nil
	}

	containerJson, err := toolKits.DockerClient.ContainerInspect(context.Background(), metadata.SandBoxID)
	if err != nil {
		klog.Errorf("can't get sandbox container %s, %v", metadata.SandBoxID, err)
		return err
	}

	// read pid limit number from label and pass to docker daemon
	for k, v := range containerJson.Config.Labels {
		if k == annotationPidsLimit {
			pidsNum, err := strconv.Atoi(v)
			if err != nil {
				klog.Errorf("invalid pid number format(%s): %v", v, err)
				return nil
			}
			klog.V(2).Infof("limit container %s/%s(%s) pid number to %d",
				metadata.Namespace, metadata.PodName, metadata.ContainerName, pidsNum)
			pidsNum64 := int64(pidsNum)
			containerConfig.InnerHostConfig.Resources.PidsLimit = &pidsNum64
			return nil
		}
	}

	return nil
}
