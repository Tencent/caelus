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

package utsmode

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/runconfig"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/docker"
)

/*
	UTS mode will create a different UTS namespace from host namespace when label is set.
This is useful when setting host network for kubernetes, but do not want to share host UTS namespace.
*/
func init() {
	docker.PreHookCreateContainer.RegisterSubPlugin(opt.handle)
}

var (
	opt = &utsMode{}
)

const (
	annotationUTSMode = plugin.PodAnnotationPrefix + "uts-mode"
)

type utsMode struct{}

func (p *utsMode) handle(
	toolKits *docker.ToolKits,
	containerConfig *runconfig.ContainerConfigWrapper,
	metadata *plugin.PodMetadata) error {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(plugin.DockerUTSMode) {
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

	// create an new UTS namespace
	for k, v := range containerJson.Config.Labels {
		if k == annotationUTSMode {
			klog.V(2).Infof("Set pod %s/%s(%s) uts mode to :%s",
				metadata.Namespace, metadata.PodName, metadata.ContainerName, v)
			containerConfig.InnerHostConfig.UTSMode = container.UTSMode(v)
		}
	}

	return nil
}
