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

package yarn

import (
	"fmt"

	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

var podInformer cache.SharedIndexInformer

const (
	// yarn config file
	YarnSite = "yarn-site.xml"

	// LocalDirs is yarn.nodemanager.local-dirs of yarn-site.
	LocalDirs = "yarn.nodemanager.local-dirs"
	// LogDirs is yarn.nodemanager.log-dirs of yarn-site.
	LogDirs = "yarn.nodemanager.log-dirs"

	subLocalDir = "yarnenv/local"
	subLogDir   = "logs/container-logs"
)

// commonResource describe common resource
type commonResource struct {
	vcores   float32
	memoryMB float32
}

// curl http://x.x.x.x:10001/jmx?qry=Hadoop:service=NodeManager,name=NodeManagerMetrics
//{
//  "beans" : [ {
//    "name" : "Hadoop:service=NodeManager,name=NodeManagerMetrics",
//    "modelerType" : "NodeManagerMetrics",
//    "tag.Context" : "yarn",
//    "tag.Hostname" : "tdw-10-120-120-84",
//    "ContainersLaunched" : 6,
//    "ContainersCompleted" : 0,
//    "ContainersFailed" : 0,
//    "ContainersKilled" : 345,
//    "ContainersIniting" : 0,
//    "ContainersRunning" : 4,
//    "AllocatedGB" : -335,
//    "AllocatedContainers" : -339,
//    "AvailableGB" : 349,
//    "AllocatedVCores" : -339,
//    "AvailableVCores" : 343,
//    "ContainerLaunchDurationNumOps" : 6,
//    "ContainerLaunchDurationAvgTime" : 49.166666666666664
//  } ]
//}

// NMMetrics is the nodemanager jmx metrics
type NMMetrics struct {
	TagHostName         string `json:"tag.Hostname"`
	ContainersLaunched  int    `json:"ContainersLaunched"`
	ContainersCompleted int    `json:"ContainersCompleted"`
	ContainersFailed    int    `json:"ContainersFailed"`
	ContainersKilled    int    `json:"ContainersKilled"`
	ContainersIniting   int    `json:"ContainersIniting"`
	ContainersRunning   int    `json:"ContainersRunning"`
	AllocatedVCores     int64  `json:"AllocatedVCores"`
	AvailableVCores     int64  `json:"AvailableVCores"`
	AllocatedGB         int64  `json:"AllocatedGB"`
	AvailableGB         int64  `json:"AvailableGB"`
}

// JmxResp is the nodemanager jmx response
type JmxResp struct {
	Beans []NMMetrics `json:"beans"`
}

/*
curl http://x.x.x.x:10001/ws/v1/node/info
{
	"nodeInfo": {
		"hadoopBuildVersion": "2.7.2 from 071ab1fd4348223d42ca1830003d75ef064db624",
		"hadoopVersion": "2.7.2",
		"hadoopVersionBuiltOn": "2019-04-16T02:49Z",
		"healthReport": "",
		"id": "10.231.146.106:8082",
		"lastNodeUpdateTime": 1597289137812,
		"nodeHealthy": true,
		"nodeHostName": "10.231.146.106",
		"nodeManagerBuildVersion": "2.7.2 from 071ab1fd4348223d42ca1830003d75ef064db624",
		"nodeManagerVersion": "2.7.2",
		"nodeManagerVersionBuiltOn": "2019-04-16T02:50Z",
		"pmemCheckEnabled": false,
		"totalPmemAllocatedContainersMB": 49664,
		"totalVCoresAllocatedContainers": 2,
		"totalVmemAllocatedContainersMB": 104294,
		"vmemCheckEnabled": false
	}
}
*/
// NMNodeInfo group nodemanager node info
type NMNodeInfo struct {
	NodeHealthy      bool    `json:"nodeHealthy"`
	NodeHealthyFloat float64 `json:"-"`
}

// NMNodeStatus describe nodemanager node status
type NMNodeStatus struct {
	NodeInfo *NMNodeInfo `json:"nodeInfo"`
}

// YarnMetadata describe meta data for YARN
type YarnMetadata struct {
	Name string
}

// AssignPodInformerValue assign value for the public variable
func AssignPodInformerValue(shareInformer cache.SharedIndexInformer) {
	podInformer = shareInformer
}

// getOfflinePod get nodemanaer pod
func getOfflinePod() (*corev1.Pod, error) {
	podList := podInformer.GetStore().List()

	var offlinePod *corev1.Pod
	podsStr := []string{}
	for _, p := range podList {
		pod := p.(*corev1.Pod)
		if appclass.IsOffline(pod) {
			offlinePod = pod
			podsStr = append(podsStr, fmt.Sprintf("%s-%s", pod.Namespace, pod.Name))
		}
	}

	if len(podsStr) == 1 {
		return offlinePod, nil
	}
	// no offline pod found
	if len(podsStr) == 0 {
		return nil, nil
	}
	// too many offline pods found, this should be not happen
	return nil, fmt.Errorf("too many offline pods, should just 1: %v", podsStr)
}
