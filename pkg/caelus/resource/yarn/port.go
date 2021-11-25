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
	"strconv"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/checkpoint"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/ports"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	yarnNodeManagerAddress          = "yarn.nodemanager.address"
	yarnNodeManagerLocalizerAddress = "yarn.nodemanager.localizer.address"
	yarnNodeManagerWebappAddress    = "yarn.nodemanager.webapp.address"

	defaultWebappAddressPort = 10001

	checkpointKey = "nm_ports"
)

var (
	// assign port for the following address automatically
	portNames = []string{yarnNodeManagerAddress, yarnNodeManagerLocalizerAddress,
		yarnNodeManagerWebappAddress}
	defaultPort = map[string]int{
		portNames[0]: 10002,
		portNames[1]: 18040,
		portNames[2]: 10001,
	}
	metricsPortKey = portNames[2]
)

// sendMetricsPort always get the right nodemanager web app port
func (g *GInit) sendMetricsPort() {
	if g.metricsPortChan != nil {
		metricsPort, err := g.GetNMWebappPort()
		if err != nil || metricsPort == nil {
			klog.Errorf("get nodemanager web app port err or nil port value: %v", err)
		} else {
			g.metricsPortChan <- *metricsPort
		}
	}
}

// ensurePort choose nodemanager port automatically
func (g *GInit) ensurePort() error {
	restorePortsCheckpoint()
	startPort := 8082
	usedPorts := sets.NewInt()
	for _, k := range portNames {
		hp := &ports.Hostport{
			Port:     defaultPort[k],
			Protocol: "tcp",
		}
		if usedPorts.Has(hp.Port) || !ports.Unused(hp) {
			unused, err := ports.FindUnusedPort(startPort, usedPorts, "tcp")
			if err != nil {
				return fmt.Errorf("ensure port of %s: %v", k, err)
			}
			hp.Port = unused
			startPort = unused + 1
		}
		usedPorts.Insert(hp.Port)
		value := fmt.Sprintf("%s:%d", util.NodeIP(), hp.Port)
		if g.metricsPortChan != nil && k == metricsPortKey {
			g.metricsPortChan <- hp.Port
		}
		var addNewKeys bool
		if m, err := g.GetProperty(YarnSite, []string{k}, false); err == nil {
			if m[k] == value {
				klog.V(5).Infof("nm conf %s=%s", k, value)
				continue
			}
			if m[k] == "" {
				addNewKeys = true
			}
		}
		if err := g.SetProperty(YarnSite, map[string]string{k: value}, addNewKeys, false); err != nil {
			return fmt.Errorf("set port property %s to %s: %v", k, value, err)
		}
		klog.V(2).Infof("setting %s=%v", k, value)
		// storing the port, and will firstly check the port at next time
		defaultPort[k] = hp.Port
	}
	storeCheckpoint()
	return nil
}

// WatchForMetricsPort watch changes of nodemanager metrics port
func (g *GInit) WatchForMetricsPort() chan int {
	if g.metricsPortChan == nil {
		g.metricsPortChan = make(chan int, 10)
	}
	return g.metricsPortChan
}

// GetNMWebappPort get nodemanager webapp port,
// return nil when getting from server failed
func (g *GInit) GetNMWebappPort() (*int, error) {
	m, err := g.GetProperty(YarnSite, []string{yarnNodeManagerWebappAddress}, false)
	if err != nil {
		return nil, err
	}
	if value, ok := m[yarnNodeManagerWebappAddress]; ok {
		portStr := strings.TrimPrefix(value, util.NodeIP()+":")
		port, err := strconv.Atoi(portStr)
		if err == nil {
			return &port, nil
		}
	} else {
		err = fmt.Errorf("key(%s) not found", yarnNodeManagerWebappAddress)
	}

	return nil, err
}

func restorePortsCheckpoint() {
	pcp := &portsCheckpoint{}
	err := checkpoint.Restore(checkpointKey, pcp)
	if err != nil {
		klog.Errorf("failed restore %s checkpoint: %v", checkpointKey, err)
		return
	}
	for _, portName := range portNames {
		if v, exist := pcp.Ports[portName]; exist && v > 0 {
			defaultPort[portName] = v
		}
	}
}

func storeCheckpoint() {
	pcp := &portsCheckpoint{Ports: defaultPort}
	if err := checkpoint.Save(checkpointKey, pcp); err != nil {
		klog.Errorf("failed store %s checkpoint: %v", checkpointKey, err)
	}
}

type portsCheckpoint struct {
	Ports map[string]int
}
