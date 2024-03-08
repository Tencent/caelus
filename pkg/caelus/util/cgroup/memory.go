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
	"io/ioutil"
	"path"
	"strconv"
	"strings"

	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs"
)

const (
	memoryLimitFile         = "memory.limit_in_bytes"
	memoryUsageFile         = "memory.usage_in_bytes"
	memoryEmptyFile         = "memory.force_empty"
	MemorySubsystem         = "memory"
	unLimitSize             = "9223372036854771712"
	memoryMoveImmigrateFile = "memory.move_charge_at_immigrate"
	// "3" indicate moving both anonymous and file pages when moving pid list
	MemoryMoveImmigrateAnonymousAndFilePages = "3"
)

// SetMemoryLimit set memory limit
func SetMemoryLimit(pathInRoot string, value int64) error {
	root := GetRoot()
	valueStr := fmt.Sprintf("%d", value)
	return WriteFile([]byte(valueStr), path.Join(root, MemorySubsystem, pathInRoot), memoryLimitFile)
}

// GetMemoryCgroupPath return memory cgroup path
func GetMemoryCgroupPath(pathInCgroup string) string {
	root := GetRoot()
	return path.Join(root, MemorySubsystem, pathInCgroup)
}

// GetMemoryLimit return memory cgroup limit value
func GetMemoryLimit(pathInRoot string) (limitValue int, limited bool, err error) {
	root := GetRoot()
	limitStr, err := readMemoryFile(path.Join(root, MemorySubsystem, pathInRoot, memoryLimitFile))
	if err != nil {
		return 0, false, err
	}

	if limitStr == unLimitSize {
		return -1, false, nil
	}

	limitValue, err = strconv.Atoi(limitStr)
	return limitValue, true, err
}

// GetMemoryUsage return memory current usage value
func GetMemoryUsage(pathInRoot string) (usageValue int, err error) {
	root := GetRoot()
	dataStr, err := readMemoryFile(path.Join(root, MemorySubsystem, pathInRoot, memoryUsageFile))
	if err != nil {
		return 0, err
	}

	usageStr := strings.Replace(dataStr, "\n", "", -1)
	return strconv.Atoi(usageStr)
}

// MemoryForceEmpty drop cache for cgroup level
func MemoryForceEmpty(pathInRoot string) (dropSupported bool, err error) {
	root := GetRoot()
	// try force empty, which just available for kernel 4.10 and later
	err = WriteFile([]byte("0"), path.Join(root, MemorySubsystem, pathInRoot), memoryEmptyFile)
	if err != nil {
		if strings.Contains(err.Error(), "write error") {
			return false, nil
		}
		return true, err
	} else {
		return true, nil
	}
}

// SetMemoryMoveImmigrate set memory.move_charge_at_immigrate for the memory cgroup.
// This file is used to move pages associated with tasks, different values have its owning meaning about what type of
// pages should be moved, as following:
//   - 0: disable moving any pages
//   - 1: moving anonymous pages used by the task
//   - 2: moving file pages mapped by the task
//   - 3: moving both anonymous pages and file pages for the task
func SetMemoryMoveImmigrate(pathInRoot, value string) error {
	root := GetRoot()
	return WriteFile([]byte(value), path.Join(root, MemorySubsystem, pathInRoot), memoryMoveImmigrateFile)
}

// GetMemoryMoveImmigrate get memory.move_charge_at_immigrate value for the memory cgroup
func GetMemoryMoveImmigrate(pathInRoot string) (string, error) {
	root := GetRoot()
	return readMemoryFile(path.Join(root, MemorySubsystem, pathInRoot, memoryMoveImmigrateFile))
}

func readMemoryFile(file string) (string, error) {
	dataBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}

	return strings.Replace(string(dataBytes), "\n", "", -1), nil
}

// GetMemoryWorkingSet get memory working set usage for givin cgroup path
func GetMemoryWorkingSet(pathInCgroup string) (int, error) {
	root := GetRoot()
	cgPath := path.Join(root, MemorySubsystem, pathInCgroup)
	g := new(fs.MemoryGroup)
	stats := cgroups.NewStats()
	if err := g.GetStats(cgPath, stats); err != nil {
		return 0, fmt.Errorf("get cpuacct cgroup stats failed: %v", err)
	}
	usage, err := GetMemoryUsage(pathInCgroup)
	if err != nil {
		return 0, err
	}
	if v, exist := stats.MemoryStats.Stats["total_inactive_file"]; exist {
		if usage < int(v) {
			usage = 0
		} else {
			usage -= int(v)
		}
	}
	return usage, nil
}
