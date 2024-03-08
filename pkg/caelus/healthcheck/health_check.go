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

package health

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/cgroupnotify"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/rulecheck"
	"github.com/tencent/caelus/pkg/caelus/qos"
	"github.com/tencent/caelus/pkg/caelus/resource"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	checkConfigFile = "/etc/caelus/rules.json"
)

// Manager is the interface for handling health check
type Manager interface {
	Name() string
	Run(stop <-chan struct{})
}

// manager detect container health based on metrics
type manager struct {
	config      *types.HealthCheckConfig
	ruleChecker *rulecheck.Manager
	// support linux kernel PSI, now just memory event
	cgroupNotifier   notify.ResourceNotify
	stStore          statestore.StateStore
	resource         resource.Interface
	qosManager       qos.Manager
	conflictMn       conflict.Manager
	podInformer      cache.SharedIndexInformer
	configUpdateFunc func(string) (*types.HealthCheckConfig, error)
	configHash       string
	globalStopCh     <-chan struct{}
}

// NewHealthManager create a new health check manager
func NewHealthManager(configFunc func(string) (*types.HealthCheckConfig, error), stStore statestore.StateStore,
	resource resource.Interface, qosManager qos.Manager, conflictMn conflict.Manager,
	podInformer cache.SharedIndexInformer) Manager {
	config, err := configFunc(checkConfigFile)
	if err != nil {
		klog.Fatalf("failed init health check config: %v", err)
	}
	hash, err := hashFile(checkConfigFile)
	if err != nil {
		klog.Fatal(err)
	}
	hm := &manager{
		config:           config,
		configUpdateFunc: configFunc,
		stStore:          stStore,
		resource:         resource,
		qosManager:       qosManager,
		conflictMn:       conflictMn,
		podInformer:      podInformer,
		ruleChecker: rulecheck.NewManager(config.RuleCheck, stStore, resource, qosManager, conflictMn,
			podInformer, config.PredictReserved),
		cgroupNotifier: notify.NewNotifyManager(&config.CgroupNotify, resource),
		configHash:     hash,
	}

	return hm
}

// Name returns the module name
func (h *manager) Name() string {
	return "ModuleHealthCheck"
}

// reload rule check config dynamically without restarting the agent
func (h *manager) reload() {
	reload, hash, config := h.checkNeedReload(checkConfigFile)
	if !reload {
		return
	}
	h.configHash = hash
	h.ruleChecker.Stop()
	h.cgroupNotifier.Stop()
	h.config = config
	h.ruleChecker = rulecheck.NewManager(config.RuleCheck, h.stStore, h.resource, h.qosManager, h.conflictMn,
		h.podInformer, config.PredictReserved)
	h.cgroupNotifier = notify.NewNotifyManager(&config.CgroupNotify, h.resource)
	go h.ruleChecker.Run(h.globalStopCh)
	go h.cgroupNotifier.Run(h.globalStopCh)
}

// checkNeedReload checks if the config file is changed
func (h *manager) checkNeedReload(configFile string) (bool, string, *types.HealthCheckConfig) {
	hash, err := hashFile(configFile)
	if err != nil {
		klog.Errorf("failed hash config file: %v", err)
		return false, "", nil
	}
	if hash == h.configHash {
		return false, "", nil
	}
	config, err := h.configUpdateFunc(configFile)
	if err != nil {
		klog.Fatalf("failed init health check config: %v", err)
	}
	if len(config.RuleNodes) != 0 {
		found := false
		for _, no := range config.RuleNodes {
			if no == util.NodeIP() {
				found = true
				break
			}
		}
		if !found {
			return false, "", nil
		}
	}

	return true, hash, config
}

// Run start checking health
func (h *manager) Run(stop <-chan struct{}) {
	if h.config.Disable {
		klog.Warningf("health check is disabled")
		return
	}
	h.globalStopCh = stop

	klog.V(2).Infof("health manager running")
	go h.ruleChecker.Run(stop)
	go h.cgroupNotifier.Run(stop)
	go h.configWatcher(stop)
}

// configWatcher support reload rule check config dynamically, no need to restart the agent
func (h *manager) configWatcher(stop <-chan struct{}) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Fatalf("failed init fsnotify watcher: %v", err)
	}
	defer w.Close()
	err = w.Add(filepath.Dir(checkConfigFile))
	if err != nil {
		klog.Fatalf("failed add dir watcher(%s): %v", filepath.Dir(checkConfigFile), err)
	}
	for {
		select {
		case <-w.Events:
			h.reload()
		case err := <-w.Errors:
			klog.Errorf("fsnotify error: %v", err)
		case <-stop:
			return
		}
	}
}

// hashFile generate hash code for the file
func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := md5.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	h := hash.Sum(nil)[:16]
	hs := hex.EncodeToString(h)
	return hs, nil
}
