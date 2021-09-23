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

package offline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/docker"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/runconfig"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog"
)

/*
	Offline is used to create the cgroup path for offline pod, which is different from kubernetes.
Then the offline pods will not managed by kubernetes, and caelus will set the cgroup value.
*/
func init() {
	docker.PreHookCreateContainer.RegisterSubPlugin(opt.mutate)
	docker.PreHookUpdateContainer.RegisterSubPlugin(opt.update)
}

var (
	opt = &offlineMutator{}
)

const (
	offlineKey = "offline"
)

type offlineMutator struct {
	startOnce  sync.Once
	toolKits   *docker.ToolKits
	allCgroups []cgroups.Mount
}

func (p *offlineMutator) mutate(
	toolKits *docker.ToolKits,
	containerConfig *runconfig.ContainerConfigWrapper,
	metadata *plugin.PodMetadata) error {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(plugin.DockerOfflineMutate) {
		return nil
	}

	// new thread to clear unused offline cgroup paths
	p.startOnce.Do(func() {
		p.toolKits = toolKits
		allCgroups, err := cgroups.GetCgroupMounts(true)
		if err != nil {
			panic(err)
		}
		p.allCgroups = allCgroups

		klog.V(2).Infof("Starting cleanup thread")
		go wait.Until(p.cleanupEmptyCgroup, time.Minute, context.Background().Done())
	})

	if metadata.ContainerType != plugin.ContainerTypeLabelContainer {
		return nil
	}

	containerJson, err := toolKits.DockerClient.ContainerInspect(context.Background(), metadata.SandBoxID)
	if err != nil {
		klog.Errorf("can't get sandbox container %s, %v", metadata.SandBoxID, err)
		return err
	}

	if !IsOffline(containerJson.Config.Labels) {
		return nil
	}

	// no need to check if cgroup parent existing
	oldCgroupParent := containerConfig.InnerHostConfig.CgroupParent
	splits := strings.Split(oldCgroupParent, string(filepath.Separator))

	if len(splits) < 3 {
		klog.V(4).Infof("splits should has 3 sections")
		return nil
	}

	newSplits := make([]string, 0, len(splits)+1)

	newSplits = append(newSplits, splits[1], offlineKey)
	newSplits = append(newSplits, splits[2:]...)
	newCgroupParent := strings.Join(newSplits, string(filepath.Separator))
	newCgroupParent = "/" + newCgroupParent

	klog.V(2).Infof("Mutate cgroup from %s to %s for pod %s/%s(%s)", oldCgroupParent, newCgroupParent,
		metadata.Namespace, metadata.PodName, metadata.ContainerName)

	// changing to a new cgroup path
	containerConfig.InnerHostConfig.CgroupParent = newCgroupParent

	return nil
}

func (p *offlineMutator) update(
	toolKits *docker.ToolKits,
	updateConfig *container.UpdateConfig,
	metadata *plugin.PodMetadata,
	containerStatus string) error {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(plugin.DockerOfflineMutate) {
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

	if !IsOffline(containerJson.Config.Labels) {
		return nil
	}

	if metadata.ContainerType != plugin.ContainerTypeLabelContainer {
		return nil
	}

	// do not allow kubernetes to set cpuset value
	if containerStatus == "created" {
		klog.V(2).Infof("Assign 0 to cpuset data for pod %s/%s(%s) with created status",
			metadata.Namespace, metadata.PodName, metadata.ContainerName)
		// bind the offline pods to cpu 0 when creating. This may be unreasonable, while not find a better way
		updateConfig.CpusetCpus = "0"
	} else {
		klog.V(4).Infof("Clear cpuset data for pod %s/%s(%s)",
			metadata.Namespace, metadata.PodName, metadata.ContainerName)
		updateConfig.CpusetCpus = ""
	}

	return nil
}

func (p *offlineMutator) cleanupEmptyCgroup() {
	containers, err := p.toolKits.DockerClient.ContainerList(context.Background(), dockertypes.ContainerListOptions{
		All: true,
	})

	if err != nil {
		klog.Exitf("failed to list containers: %v", err)
	}

	existedUids := sets.NewString()
	for _, c := range containers {
		if len(c.Labels) > 0 {
			if uid, ok := c.Labels[plugin.PodUIDLabelKey]; ok {
				existedUids.Insert(uid)
			}
		}
	}

	cleanPaths := make([]string, 0)
	for _, cg := range p.allCgroups {
		filepath.Walk(cg.Mountpoint, func(path string, info os.FileInfo, err error) error {
			// skip controller file
			if !info.IsDir() {
				return nil
			}

			// skip container cgroup directory
			if !checkCleaningCgroupAvailable(info.Name(), path, cg.Mountpoint) {
				return nil
			}

			podUid := strings.TrimPrefix(info.Name(), "pod")
			if !existedUids.Has(podUid) {
				klog.V(2).Infof("Clean cgroup %s", path)
				cleanPaths = append(cleanPaths, path)
			}
			return nil
		})
	}

	for _, p := range cleanPaths {
		if err := os.RemoveAll(p); err == nil {
			klog.V(3).Infof("Cleaned %s", p)
		} else {
			klog.V(3).Infof("can't clean %s, %v", p, err)
		}
	}
}

// check cgroup path, such as:
// /sys/fs/cgroup/cpuset/kubepods/offline/besteffort/podf8f39b96-8198-43da-ac67-40f339d8441a
func checkCleaningCgroupAvailable(name, path, mountPoint string) bool {
	if len(name) == 39 && strings.HasPrefix(name, "pod") &&
		strings.Contains(path, offlineKey) {

		// noMnPoint, like /kubepods/offline/besteffort/podxxxx
		noMnPoint := strings.TrimPrefix(path, mountPoint)
		splits := strings.Split(noMnPoint, string(filepath.Separator))
		if len(splits) > 3 && splits[2] == offlineKey {
			return true
		}
	}

	return false
}
