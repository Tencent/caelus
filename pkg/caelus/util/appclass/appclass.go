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

package appclass

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	AnnotationOfflineKey   = types.PodAnnotationPrefix + "app-class"
	AnnotationOfflineValue = "greedy"
)

// copied from kubernetes/pkg/apis/core/v1/helper/qos/qos.go to not import kubernetes
var supportedQoSComputeResources = sets.NewString(string(v1.ResourceCPU), string(v1.ResourceMemory))

// QOSList is a set of (resource name, QoS class) pairs.
type QOSList map[v1.ResourceName]v1.PodQOSClass

// IsOffline return if a pod is offline pod
// if offline is nodemanager, should be added the annotation key with greedy
func IsOffline(pod *v1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	if value, ok := pod.Annotations[AnnotationOfflineKey]; ok {
		if value == AnnotationOfflineValue {
			return true
		}
	}
	return false
}

// PodCgroupDirs get pod cgroup path
func PodCgroupDirs(pod *v1.Pod) []string {
	cgroupPrefix := types.CgroupKubePods
	// cgroup path for offline has changed to /kubepods/offline, like: /kubepods/offline/besteffort/podxx/xx
	if IsOffline(pod) {
		cgroupPrefix = types.CgroupOffline
	}

	class := pod.Status.QOSClass
	var ret []string
	switch class {
	case v1.PodQOSBestEffort:
		fallthrough
	case v1.PodQOSBurstable:
		cgroupPrefix = path.Join(cgroupPrefix, strings.ToLower(string(class)))
	case v1.PodQOSGuaranteed:
	default:
		return []string{}
	}

	podDir := filepath.Join(cgroupPrefix, "pod"+string(pod.UID))
	for _, cs := range pod.Status.ContainerStatuses {
		// the docker and containerd runtime has different prefix string, just to trim both and no need to check
		cid := strings.TrimPrefix(cs.ContainerID, "docker://")
		cid = strings.TrimPrefix(cid, "containerd://")
		// cid may be empty when pod is in creating status
		if len(cid) != 0 {
			ret = append(ret, filepath.Join(podDir, cid))
		} else {
			klog.Warningf("no container id found, pod(%s-%s) may be in creating status",
				pod.Namespace, pod.Name)
		}
	}
	return ret
}

// AppClass defines
type AppClass string

const (
	// oneline pod
	AppClassOnline = "online"
	// offline pod
	AppClassOffline = "offline"
	// system pod
	AppClassSystem  = "system"
	AppClassUnknown = "unknown"
	// such as /kubepods/burstable
	AppClassAggregatedCgroup = "aggregatedcgroup"
)

// GetAppClass return app class of this pod
func GetAppClass(pod *v1.Pod) AppClass {
	if pod.Namespace == "kube-system" {
		return AppClassSystem
	}
	if pod.Annotations == nil {
		return AppClassUnknown
	}
	if value, ok := pod.Annotations[AnnotationOfflineKey]; ok {
		if value == AnnotationOfflineValue {
			return AppClassOffline
		}
	}
	return AppClassOnline
}
