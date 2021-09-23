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

package types

import (
	"time"

	"github.com/tencent/caelus/pkg/cadvisor"

	"github.com/google/cadvisor/cache/memory"
	cadvisormetrics "github.com/google/cadvisor/container"
	"github.com/google/cadvisor/utils/sysfs"
)

const (
	// EnvPort show server port
	EnvPort = "GINIT_PORT"
)

var (
	// cadvisorMetrics describe which metrics need to collect, just collecting cpu and memory
	cadvisorMetrics = cadvisormetrics.MetricSet{
		"cpu":    struct{}{},
		"memory": struct{}{},
	}
	cadvisorCacheDuration           = 2 * time.Minute
	cadvisorMaxHousekeepingInterval = 15 * time.Second

	HadoopPath = "/hadoop-yarn"
	CgroupRoot = "/sys/fs/cgroup"

	// CgroupPath describe witch cgroup path to collect
	CgroupPath = []string{HadoopPath}

	CadvisorParameters = cadvisor.CadvisorParameter{
		MemCache:                memory.New(cadvisorCacheDuration, nil),
		SysFs:                   sysfs.NewRealSysFs(),
		IncludeMetrics:          cadvisorMetrics,
		MaxHousekeepingInterval: cadvisorMaxHousekeepingInterval,
	}
)

// RMAppWrapper show the applications struct from nodemanager API
type RMAppWrapper struct {
	App RMApp `json:"app"`
}

// RMApp describe application options
type RMApp struct {
	ID          string  `json:"id"`
	User        string  `json:"user"`
	Name        string  `json:"name"`
	Queue       string  `json:"queue"`
	State       string  `json:"state"`
	FinalStatus string  `json:"finalStatus"`
	Progress    float32 `json:"progress"`
	TrackingUI  string  `json:"trackingUI"`
	AmLogs      string  `json:"amContainerLogs"`
}
