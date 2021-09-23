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

package manager

import (
	"fmt"

	"k8s.io/api/core/v1"
)

// ResourceQosManager describe resource manager interface functions
type ResourceQosManager interface {
	Name() string
	PreInit() error
	Run(stop <-chan struct{})
	Manage(cgResources *CgroupResourceConfig) error
}

// CgroupResourceConfig group options for offline pods
type CgroupResourceConfig struct {
	OnlineCgroups  []string
	OfflineCgroups []string
	Resources      v1.ResourceList
	PodList        []*v1.Pod
}

// String formats cgroup resource output
func (c *CgroupResourceConfig) String() string {
	pods := []string{}
	for _, pod := range c.PodList {
		pods = append(pods, fmt.Sprintf("%s-%s", pod.Namespace, pod.Name))
	}
	return fmt.Sprintf("pods: %v, onlineCgroups: %v, offlineCgroups: %v,"+
		" resources: %v", pods, c.OnlineCgroups, c.OfflineCgroups, c.Resources)
}
