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
	"os"
	"time"

	"github.com/tencent/caelus/pkg/caelus/qos/manager/netio"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	QosNetIO = "netio"
	EnvEniIP = "ENI_IP_RESOURCE"
)

var eniIPResource v1.ResourceName

type qosNetIO struct {
	// eni network traffic via eth0, host and global route network traffic via eth1, so we need two TCShaper
	iface, eniIface string
	tc, enitc       *netio.EgressShaper
}

// NewQosNetIO creates network io manager instance
func NewQosNetIO(iface, eniIface string) ResourceQosManager {
	var err error
	var tc, enitc *netio.EgressShaper
	if iface != "" {
		tc, err = netio.NewEgressShaper(iface)
		if err != nil {
			klog.Fatalf("new egress shaper: %v", err)
		}
	}
	if eniIface != "" {
		enitc, err = netio.NewEgressShaper(eniIface)
		if err != nil {
			klog.Fatalf("new eni egress shaper: %v", err)
		}
	}

	return &qosNetIO{
		iface:    iface,
		eniIface: eniIface,
		tc:       tc,
		enitc:    enitc,
	}
}

// Name returns resource policy name
func (n *qosNetIO) Name() string {
	return "netio"
}

// PreInit do nothing
func (n *qosNetIO) PreInit() error {
	return nil
}

// Run starts setting tc shaper periodically
func (n *qosNetIO) Run(stop <-chan struct{}) {
	if n.tc != nil {
		klog.V(2).Infof("start ReconcileInterface for iface %s", n.iface)
		go wait.Until(func() {
			if err := n.tc.ReconcileInterface(); err != nil {
				klog.Error(err)
			}
		}, time.Minute*3, stop)
	}
	if n.enitc != nil {
		klog.V(2).Infof("start ReconcileInterface for eni iface %s", n.eniIface)
		go wait.Until(func() {
			if err := n.enitc.ReconcileInterface(); err != nil {
				klog.Error(err)
			}
		}, time.Minute*3, stop)
	}
}

// ManageNetIO isolates network io resource for offline jobs
func (n *qosNetIO) Manage(cgResources *CgroupResourceConfig) error {
	var eniIPs, globalRouteIPs, hostNetworkCgroupPaths []string

	for _, pod := range cgResources.PodList {
		if pod.Namespace == "kube-system" || !appclass.IsOffline(pod) {
			continue
		}
		if pod.Spec.HostNetwork {
			hostNetworkCgroupPaths = append(hostNetworkCgroupPaths, appclass.PodCgroupDirs(pod)...)
			continue
		}
		if pod.Status.PodIP == "" {
			continue
		}
		if eniPod(pod) {
			eniIPs = append(eniIPs, pod.Status.PodIP)
			continue
		}
		globalRouteIPs = append(globalRouteIPs, pod.Status.PodIP)
	}

	if n.tc != nil {
		if err := n.tc.EnsureEgressOfCgroup(hostNetworkCgroupPaths); err != nil {
			klog.Warningf("ensure egress: %v", err)
		}
		if err := n.tc.EnsureEgressOfIPs(globalRouteIPs); err != nil {
			klog.Warningf("ensure egress: %v", err)
		}
	}
	if n.enitc != nil {
		if err := n.enitc.EnsureEgressOfIPs(eniIPs); err != nil {
			klog.Warningf("ensure egress: %v", err)
		}
	}
	return nil
}

// eniPod check if pod is floating ip
func eniPod(pod *v1.Pod) bool {
	if len(eniIPResource) == 0 {
		eniIPResource = v1.ResourceName(os.Getenv(EnvEniIP))
	}

	for i := range pod.Spec.Containers {
		reqResource := pod.Spec.Containers[i].Resources.Requests
		for name := range reqResource {
			if name == eniIPResource {
				return true
			}
		}
	}
	return false
}
