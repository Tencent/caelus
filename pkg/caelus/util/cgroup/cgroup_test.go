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
	"os"
	"path"
	"testing"

	"golang.org/x/sys/unix"
)

var (
	testSourceCgroup = "test_source"
	testTargetCgroup = "test_target"
	cgroupProc       = "cgroup.procs"
)

// TestMovePids test moving pid list from source cgroup to target cgroup for assigned cgroup subsystems
func TestMovePids(t *testing.T) {
	rootPath := GetRoot()

	prepareCgoupPath(rootPath)
	defer func() {
		MovePids([]string{CpuSetSubsystem, MemorySubsystem}, testTargetCgroup, "")
		destoryCgroupPath(rootPath)
	}()

	pid := unix.Getpid()

	err := WriteFile([]byte(fmt.Sprintf("%d", pid)),
		path.Join(rootPath, CpuSetSubsystem, testSourceCgroup), cgroupProc)
	if err != nil {
		t.Skipf("write pid to cgroup(%s) failed: %v", CpuSetSubsystem, err)
	}
	err = WriteFile([]byte(fmt.Sprintf("%d", pid)),
		path.Join(rootPath, MemorySubsystem, testSourceCgroup), cgroupProc)
	if err != nil {
		t.Skipf("write pid to cgroup(%s) failed: %v", MemorySubsystem, err)
	}

	_, err = MovePids([]string{CpuSetSubsystem, MemorySubsystem}, testSourceCgroup, testTargetCgroup)
	if err != nil {
		t.Fatalf("test moving cgroup pids from %s to %s failed: %v",
			testSourceCgroup, testTargetCgroup, err)
	}

	newPids, err := GetPids(path.Join(rootPath, CpuSetSubsystem, testTargetCgroup))
	if err != nil {
		t.Fatalf("get pid from target cgroup(%s-%s) failed: %v",
			CpuSetSubsystem, testTargetCgroup, err)
	}
	if err := comparePid(pid, newPids); err != nil {
		t.Fatalf("test moving cgroup pids failed: %v", err)
	}
	newPids, err = GetPids(path.Join(rootPath, MemorySubsystem, testTargetCgroup))
	if err != nil {
		t.Fatalf("get pid from target cgroup(%s-%s) failed: %v",
			MemorySubsystem, testTargetCgroup, err)
	}
	if err := comparePid(pid, newPids); err != nil {
		t.Fatalf("test moving cgroup pids failed: %v", err)
	}
}

// TestMoveSpecificPids test moving pid list to target cgroup for assigned cgroup subsystems
func TestMoveSpecificPids(t *testing.T) {
	rootPath := GetRoot()

	prepareCgoupPath(rootPath)
	defer func() {
		MovePids([]string{CpuSetSubsystem, MemorySubsystem}, testTargetCgroup, "")
		destoryCgroupPath(rootPath)
	}()

	pid := unix.Getpid()
	err := MoveSpecificPids([]string{CpuSetSubsystem, MemorySubsystem}, []int{pid}, testTargetCgroup)
	if err != nil {
		t.Fatalf("test moving specific cgroup pid list to %s failed: %v", testTargetCgroup, err)
	}

	newPids, err := GetPids(path.Join(rootPath, CpuSetSubsystem, testTargetCgroup))
	if err != nil {
		t.Fatalf("get pid from target cgroup(%s-%s) failed: %v",
			CpuSetSubsystem, testTargetCgroup, err)
	}
	if err := comparePid(pid, newPids); err != nil {
		t.Fatalf("test moving specific cgroup pid list failed: %v", err)
	}

	newPids, err = GetPids(path.Join(rootPath, MemorySubsystem, testTargetCgroup))
	if err != nil {
		t.Fatalf("get pid from target cgroup(%s-%s) failed: %v",
			MemorySubsystem, testTargetCgroup, err)
	}
	if err := comparePid(pid, newPids); err != nil {
		t.Fatalf("test moving specific cgroup pid list failed: %v", err)
	}
	// special check for memory move charge immigrate setting
	memoryMoveImmigrateValue, err := GetMemoryMoveImmigrate(testTargetCgroup)
	if err != nil {
		t.Fatalf("get memory move immigrate value from target cgroup(%s) failed: %v",
			testTargetCgroup, err)
	}
	if memoryMoveImmigrateValue != MemoryMoveImmigrateAnonymousAndFilePages {
		t.Fatalf("get memory move immigrate value from target cgroup(%s) unexpected, should be %s, got %s",
			testTargetCgroup, MemoryMoveImmigrateAnonymousAndFilePages, memoryMoveImmigrateValue)
	}
}

func prepareCgoupPath(rootPath string) {
	os.MkdirAll(path.Join(rootPath, CpuSetSubsystem, testSourceCgroup), 0700)
	os.MkdirAll(path.Join(rootPath, CpuSetSubsystem, testTargetCgroup), 0700)
	EnsureCpuSetCores(testSourceCgroup)
	EnsureCpuSetCores(testTargetCgroup)
	os.MkdirAll(path.Join(rootPath, MemorySubsystem, testSourceCgroup), 0700)
	os.MkdirAll(path.Join(rootPath, MemorySubsystem, testTargetCgroup), 0700)
}

func destoryCgroupPath(rootPath string) {
	os.RemoveAll(path.Join(rootPath, CpuSetSubsystem, testSourceCgroup))
	os.RemoveAll(path.Join(rootPath, CpuSetSubsystem, testTargetCgroup))
	os.RemoveAll(path.Join(rootPath, MemorySubsystem, testSourceCgroup))
	os.RemoveAll(path.Join(rootPath, MemorySubsystem, testTargetCgroup))

}

func comparePid(originPid int, newPids []int) error {
	if len(newPids) == 0 {
		return fmt.Errorf("new pids is nil")
	}
	if len(newPids) > 1 {
		return fmt.Errorf("new pids too many: %v", newPids)
	}

	if originPid != newPids[0] {
		return fmt.Errorf("pid not same, should be %d, but get %d", originPid, newPids[0])
	}

	return nil
}
