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
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/opencontainers/runc/libcontainer/cgroups"
	"k8s.io/klog/v2"
)

var cachedCgroupRoot unsafe.Pointer

const (
	DevicesSubsystem = "devices"
)

// GetRoot get cgroup root path
func GetRoot() string {
	if root := (*string)(atomic.LoadPointer(&cachedCgroupRoot)); root != nil {
		return *root
	}
	root, err := findCgroupMountpointDir()
	if err != nil {
		klog.Fatalf("failed to find cgroup mount point dir")
	}
	// root should be /sys/fs/cgroup
	if _, err := os.Stat(root); err != nil {
		klog.Fatalf("cgroup mount point not exists %s", root)
	}
	atomic.StorePointer(&cachedCgroupRoot, unsafe.Pointer(&root))
	return root
}

// findCgroupMountpointDir is originally existed in old version runc, but removed in new version, so dump code here
func findCgroupMountpointDir() (string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		fields := strings.Split(text, " ")
		// Safe as mountinfo encodes mountpoints with spaces as \040.
		index := strings.Index(text, " - ")
		postSeparatorFields := strings.Fields(text[index+3:])
		numPostFields := len(postSeparatorFields)

		// This is an error as we can't detect when the mount is for "cgroup"
		if numPostFields == 0 {
			return "", fmt.Errorf("Found no fields post '-' in %q", text)
		}

		if postSeparatorFields[0] == "cgroup" {
			// Check that the mount is properly formated.
			if numPostFields < 3 {
				return "", fmt.Errorf("Error found less than 3 fields post '-' in %q", text)
			}

			return filepath.Dir(fields[4]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("not found")
}

// GetPids get cgroup pid list
func GetPids(fullCgPath string) ([]int, error) {
	return cgroups.GetPids(fullCgPath)
}

// WriteFile write value to cgroup file
func WriteFile(data []byte, cgroupPath, cgroupFile string) error {
	return ioutil.WriteFile(filepath.Join(cgroupPath, cgroupFile), data, 0664)
}

// EnsureCgroupPath check if cgroup path existed
func EnsureCgroupPath(pathInRoot string) error {
	cgs, err := cgroups.GetAllSubsystems()
	if err != nil {
		return err
	}

	rootPath := GetRoot()
	for _, cg := range cgs {
		os.MkdirAll(path.Join(rootPath, cg, pathInRoot), 0700)
	}

	return nil
}

// MovePids move pids of subsystems from source path to target path
func MovePids(subsystems []string, sourePath, targetPath string) ([]int, error) {
	if len(subsystems) == 0 {
		return nil, nil
	}

	rootPath := GetRoot()
	var errs []error
	var movedPids []int
	for _, sub := range subsystems {
		if sub == MemorySubsystem {
			err := SetMemoryMoveImmigrate(targetPath, MemoryMoveImmigrateAnonymousAndFilePages)
			if err != nil {
				return movedPids, fmt.Errorf("set memory moving immigrate for path %s err: %v", targetPath, err)
			}
		}
		fullSPath := path.Join(rootPath, sub, sourePath)
		pids, err := GetPids(fullSPath)
		if err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, err)
			}
			continue
		}
		if len(pids) == 0 {
			continue
		}
		movedPids = append(movedPids, pids...)

		fullTPath := path.Join(rootPath, sub, targetPath, "cgroup.procs")
		for i := range pids {
			if err := ioutil.WriteFile(fullTPath,
				[]byte(strconv.Itoa(pids[i])), 0664); err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}

	if len(errs) != 0 {
		return movedPids, fmt.Errorf("write pid err: %+v", errs)
	}

	return movedPids, nil
}

// MoveSpecificPids moves specific pids to target cgroup path with assigned subsystem
func MoveSpecificPids(subsystems []string, pids []int, targetPath string) error {
	if len(subsystems) == 0 || len(pids) == 0 {
		return nil
	}

	rootPath := GetRoot()
	var errs []error
	for _, sub := range subsystems {
		fullTPath := path.Join(rootPath, sub, targetPath, "cgroup.procs")
		if sub == MemorySubsystem {
			err := SetMemoryMoveImmigrate(targetPath, MemoryMoveImmigrateAnonymousAndFilePages)
			if err != nil {
				return fmt.Errorf("set memory moving immigrate for path %s err: %v", targetPath, err)
			}
		}
		for i := range pids {
			if err := ioutil.WriteFile(fullTPath,
				[]byte(strconv.Itoa(pids[i])), 0664); err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("write pid err: %+v", errs)
	}

	return nil
}

// GetCgroupsByPid parses /proc/<pid>/cgroup file into a map of subgroups to cgroup names.
func GetCgroupsByPid(pid int) (map[string]string, error) {
	return cgroups.ParseCgroupFile(fmt.Sprintf("/proc/%d/cgroup", pid))
}
