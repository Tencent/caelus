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

package storageopts

import (
	"context"
	"strings"

	"github.com/docker/docker/runconfig"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/docker"
)

/*
	Storage opts will set container root folder disk space limit size to docker daemon based on pod label.
*/
func init() {
	docker.PreHookCreateContainer.RegisterSubPlugin(opt.handle)
}

var (
	opt = &storageOption{}
)

const (
	optionPrefix = plugin.PodAnnotationPrefix + "storage-opt-"
)

type storageOption struct{}

func (p *storageOption) handle(
	toolKits *docker.ToolKits,
	containerConfig *runconfig.ContainerConfigWrapper,
	metadata *plugin.PodMetadata) error {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(plugin.DockerStorageOption) {
		return nil
	}

	if metadata.ContainerType != plugin.ContainerTypeLabelContainer {
		return nil
	}

	if containerConfig.InnerHostConfig.StorageOpt == nil {
		containerConfig.InnerHostConfig.StorageOpt = make(map[string]string)
	}

	containerJson, err := toolKits.DockerClient.ContainerInspect(context.Background(), metadata.SandBoxID)
	if err != nil {
		klog.Errorf("can't get sandbox container %s, %v", metadata.SandBoxID, err)
		return err
	}

	// the keys must be supported by docker daemon, such as storage-opt-size = xxx
	for k, v := range containerJson.Config.Labels {
		if strings.HasPrefix(k, optionPrefix) {
			if optKey := strings.TrimPrefix(k, optionPrefix); len(optKey) > 0 {
				klog.V(2).Infof("Set pod %s/%s(%s) storage option %s=%s",
					metadata.Namespace, metadata.PodName, metadata.ContainerName, optKey, v)
				containerConfig.InnerHostConfig.StorageOpt[optKey] = v
			}
		}
	}

	return nil
}
