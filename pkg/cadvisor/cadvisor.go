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

package cadvisor

import (
	"flag"
	"net/http"
	"time"

	"github.com/google/cadvisor/cache/memory"
	cadvisormetrics "github.com/google/cadvisor/container"
	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"github.com/google/cadvisor/manager"
	"github.com/google/cadvisor/utils/sysfs"
	"k8s.io/klog"
)

func init() {
	// Override cAdvisor flag defaults.
	flagOverrides := map[string]string{
		// Override the default cAdvisor housekeeping interval.
		"housekeeping_interval": "10s",
	}
	for name, defaultValue := range flagOverrides {
		if f := flag.Lookup(name); f != nil {
			f.DefValue = defaultValue
			f.Value.Set(defaultValue)
		} else {
			klog.Errorf("Expected cAdvisor flag %q not found", name)
		}
	}
}

// CadvisorParameter describe options for cadvisor
type CadvisorParameter struct {
	MemCache                *memory.InMemoryCache
	SysFs                   sysfs.SysFs
	IncludeMetrics          cadvisormetrics.MetricSet
	MaxHousekeepingInterval time.Duration
}

// Cadvisor interface
type Cadvisor interface {
	Start() error
	Stop() error
	MachineInfo() (*cadvisorapi.MachineInfo, error)
	ContainerInfoV2(name string,
		options cadvisorapiv2.RequestOptions) (map[string]cadvisorapiv2.ContainerInfo, error)
	ContainerSpec(containerName string, options cadvisorapiv2.RequestOptions) (
		map[string]cadvisorapiv2.ContainerSpec, error)
}

type cmanager struct {
	manager.Manager
}

// NewCadvisorManager creates a new cadvisor manager instance
func NewCadvisorManager(cadPram CadvisorParameter, cgroupPrefixs []string) Cadvisor {
	m, err := manager.New(cadPram.MemCache, cadPram.SysFs, cadPram.MaxHousekeepingInterval,
		true, cadPram.IncludeMetrics, http.DefaultClient, cgroupPrefixs)
	if err != nil {
		klog.Fatalf("cadvisor manager start err: %v", err)
	}
	return &cmanager{
		Manager: m,
	}
}

// Start start cadvisor manager
func (c *cmanager) Start() error {
	return c.Manager.Start()
}

// Stop stop cadvisor and clear existing factory
func (c *cmanager) Stop() error {
	err := c.Manager.Stop()
	if err != nil {
		return err
	}

	// clear existing factory
	cadvisormetrics.ClearContainerHandlerFactories()

	return nil
}

// MachineInfo get machine metrics
func (c *cmanager) MachineInfo() (*cadvisorapi.MachineInfo, error) {
	return c.GetMachineInfo()
}

// ContainerInfo get container infos v2
func (c *cmanager) ContainerInfoV2(name string,
	options cadvisorapiv2.RequestOptions) (map[string]cadvisorapiv2.ContainerInfo, error) {
	return c.GetContainerInfoV2(name, options)
}

// ContainerSpec get container spec
func (c *cmanager) ContainerSpec(containerName string, options cadvisorapiv2.RequestOptions) (
	map[string]cadvisorapiv2.ContainerSpec, error) {
	return c.GetContainerSpec(containerName, options)
}
