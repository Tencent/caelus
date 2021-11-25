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

package k8s

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
)

const (
	ExternalResourcePrefix = "mixer.kubernetes.io/ext-"
)

// NodeInfo group node level info
type NodeInfo struct {
	// total requested extended resources of all pods on this node
	RequestedResource v1.ResourceList
}

// NewNodeInfo new a NodeInfo object.
// If any pods are given in arguments, their extended resources will be accumulated.
func NewNodeInfo(pods ...*v1.Pod) *NodeInfo {
	ni := &NodeInfo{
		RequestedResource: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(0, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(0, resource.DecimalSI),
		},
	}

	for _, pod := range pods {
		ni.AddPod(pod)
	}
	return ni
}

// ResetNodeInfo clear the NodeInfo object.
// If any pods are given in arguments, their extended resources will be accumulated.
func (n *NodeInfo) ResetNodeInfo(pods ...*v1.Pod) {
	n.RequestedResource = v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(0, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(0, resource.DecimalSI),
	}

	for _, pod := range pods {
		n.AddPod(pod)
	}
}

// AddPod accumulate pod's extended resources to the NodeInfo
func (n *NodeInfo) AddPod(pod *v1.Pod) {
	res := GetPodResourceRequest(pod)
	for rName, rQuanty := range res {
		rqQ := n.RequestedResource[rName].DeepCopy()
		rqQ.Add(rQuanty)
		n.RequestedResource[rName] = rqQ
	}
}

// the function check if the extended request resources are more than resources given in parameter
func (n *NodeInfo) More(rl v1.ResourceList) (bool, v1.ResourceName) {
	for k, v := range n.RequestedResource {
		if vl, ok := rl[k]; ok {
			if v.Cmp(vl) > 0 {
				return true, k
			}
		}
	}

	return false, ""
}

// the function check if the extended request resource are less than resources given in parameter
func (n *NodeInfo) Less(rl v1.ResourceList) bool {
	for k, v := range n.RequestedResource {
		if vl, ok := rl[k]; ok {
			if v.Cmp(vl) >= 0 {
				return false
			}
		}
	}

	return true
}

// ReduceRequestedResource take off the resource quantity from node's total request resource quantity
func (n *NodeInfo) ReduceRequestedResource(resQuan v1.ResourceList) {
	for r, q := range resQuan {
		req, ok := n.RequestedResource[r]
		if !ok {
			continue
		}
		req.Sub(q)
		n.RequestedResource[r] = req
	}
}

// GetPodResourceRequest output pod's extended request resource quantity
func GetPodResourceRequest(pod *v1.Pod) v1.ResourceList {
	// just for cpu and memory
	requestList := v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(0, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(0, resource.DecimalSI),
	}

	for _, c := range pod.Spec.Containers {
		for rName, rQuanty := range c.Resources.Requests {
			if v1helper.IsExtendedResourceName(rName) {
				rNameNoPrefix := v1.ResourceName(strings.TrimPrefix(string(rName), ExternalResourcePrefix))
				if vv, ok := requestList[rNameNoPrefix]; ok {
					if rNameNoPrefix == v1.ResourceCPU {
						newQuanty := resource.NewMilliQuantity(rQuanty.Value(), resource.DecimalSI)
						vv.Add(*newQuanty)
					} else {
						vv.Add(rQuanty)
					}
					requestList[rNameNoPrefix] = vv
				}
			}
		}
	}

	return requestList
}

// GetPodKey output the key identification for pod, which is namespace and pod name
func GetPodKey(pod *v1.Pod) string {
	return fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
}
