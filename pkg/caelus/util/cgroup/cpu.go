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

package cgroup

import (
	"fmt"
	"path"
	"strconv"

	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs"
)

const (
	CPUSubsystem = "cpu"
)

// GetCPUTotalUsage get cpu usage for cgroup
func GetCPUTotalUsage(pathInCgroup string) (uint64, error) {
	root := GetRoot()
	cgPath := path.Join(root, CPUSubsystem, pathInCgroup)
	g := new(fs.CpuacctGroup)
	stats := cgroups.NewStats()
	if err := g.GetStats(cgPath, stats); err != nil {
		return 0, fmt.Errorf("get cpuacct cgroup stats failed: %v", err)
	}
	return stats.CpuStats.CpuUsage.TotalUsage, nil
}

// SetCPUShares set cpu.shares for cgroup
func SetCPUShares(pathInCgroup string, value uint64) error {
	root := GetRoot()
	return WriteFile([]byte(strconv.FormatUint(value, 10)),
		path.Join(root, CPUSubsystem, pathInCgroup), "cpu.shares")
}

// SetCpuQuota set cpu.cfs_quota_us for cgroup
func SetCpuQuota(pathInCgroup string, cores float64) error {
	root := GetRoot()
	// changing default value 100000 to 1000000 for cpu.cfs_period_us
	err := WriteFile([]byte("1000000"),
		path.Join(root, CPUSubsystem, pathInCgroup), "cpu.cfs_period_us")
	if err != nil {
		return err
	}
	value := int64(cores * 1000000)
	if cores == -1 {
		value = -1
	}
	return WriteFile([]byte(strconv.FormatInt(value, 10)),
		path.Join(root, CPUSubsystem, pathInCgroup), "cpu.cfs_quota_us")
}
