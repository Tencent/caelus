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
	"io/ioutil"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opencontainers/runc/libcontainer/cgroups"
)

const (
	CpuSetSubsystem = "cpuset"
	cpusetCpus      = "cpuset.cpus"
	cpusetMems      = "cpuset.mems"
)

// GetCpuSet return cores limited for the cgroup, with parsed format or origin value
func GetCpuSet(pathInRoot string, needParse bool) ([]string, error) {
	var cpus []string

	rootPath := GetRoot()
	data, err := ioutil.ReadFile(filepath.Join(rootPath, CpuSetSubsystem, pathInRoot, "cpuset.cpus"))
	if err != nil {
		return cpus, err
	}
	cpusetStr := strings.Replace(string(data), "\n", "", -1)

	if !needParse {
		if len(cpusetStr) != 0 {
			cpus = append(cpus, cpusetStr)
		}
		return cpus, nil
	}

	cpusets := strings.Split(cpusetStr, ",")
	for _, v := range cpusets {
		index := strings.Index(v, "-")
		if index == -1 {
			cpus = append(cpus, v)
		} else {
			min, _ := strconv.Atoi(v[:index])
			max, _ := strconv.Atoi(v[index+1:])
			for j := min; j <= max; j++ {
				cpus = append(cpus, strconv.Itoa(j))
			}
		}
	}

	return cpus, nil
}

// GetCpuSetPids will get pids from "cgroup.procs"
func GetCpuSetPids(pathInRoot string) ([]int, error) {
	rootPath := GetRoot()
	fullCgPath := path.Join(rootPath, CpuSetSubsystem, pathInRoot)
	return cgroups.GetPids(fullCgPath)
}

// EnsureCpuSetCores will check if the "cpuset.cores" is nil, and assign global cores if is nil.
func EnsureCpuSetCores(pathInRoot string) error {
	rootPath := GetRoot()
	originCores, err := ioutil.ReadFile(path.Join(rootPath, CpuSetSubsystem, pathInRoot, cpusetCpus))
	if err != nil {
		return err
	}
	// if it has already assigned the cores, just return
	if string(originCores) != "\n" {
		return nil
	}

	totalCores, err := ioutil.ReadFile(path.Join(rootPath, CpuSetSubsystem, cpusetCpus))
	if err != nil {
		return err
	}
	totalMems, err := ioutil.ReadFile(path.Join(rootPath, CpuSetSubsystem, cpusetMems))
	if err != nil {
		return err
	}

	pathInRoot = strings.TrimPrefix(pathInRoot, "/")
	parts := strings.Split(pathInRoot, "/")
	for i := 0; i < len(parts); i++ {
		subpath := path.Join(parts[:i+1]...)
		err = WriteFile(totalCores, path.Join(rootPath, CpuSetSubsystem, subpath), cpusetCpus)
		if err != nil {
			return err
		}

		err = WriteFile(totalMems, path.Join(rootPath, CpuSetSubsystem, subpath), cpusetMems)
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteCpuSetCores write cores to "cpuset.cpus"
func WriteCpuSetCores(pathInRoot string, cores []int) error {
	if len(cores) == 0 {
		return nil
	}

	rootPath := GetRoot()
	coresStr := ""
	for _, c := range cores {
		coresStr = coresStr + strconv.Itoa(c) + ","
	}
	coresStr = strings.TrimSuffix(coresStr, ",")

	return WriteFile([]byte(coresStr), path.Join(rootPath, CpuSetSubsystem, pathInRoot), cpusetCpus)
}

// WriteCpuSetCoresStr write cores in string format to "cpuset.cpus"
func WriteCpuSetCoresStr(pathInRoot string, coresStr string) error {
	if len(coresStr) == 0 {
		return nil
	}

	rootPath := GetRoot()
	return WriteFile([]byte(coresStr), path.Join(rootPath, CpuSetSubsystem, pathInRoot), cpusetCpus)

}
