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
	"os"
	"path"
	"strings"
	"testing"
)

// TestBlkioLimitSet test blkio limit
func TestBlkioLimitSet(t *testing.T) {
	root := GetRoot()

	subsysteCgroup := "/docker"
	cgroupPath := path.Join(root, "blkio", subsysteCgroup)

	_, err := os.Stat(cgroupPath)
	if err != nil {
		if err = os.MkdirAll(cgroupPath, 0755); err != nil {
			t.Skipf("Create blkio cgroup failed")
		}
	}

	disks, err := ioutil.ReadDir(blockDir)
	if err != nil {
		t.Skipf("Read disks from %v failed", blockDir)
	}

	targetdisk := ""
	for _, disk := range disks {
		if strings.Contains(disk.Name(), "da") {
			targetdisk = disk.Name()
			break
		}
	}
	if targetdisk == "" {
		t.Skipf("Cannot find disk from %v", blockDir)
	}

	writeKbps := map[string]uint64{
		targetdisk: 20480,
	}
	readKbps := map[string]uint64{
		targetdisk: 10240,
	}

	BlkioLimitSet(subsysteCgroup, writeKbps, readKbps)
	writevalue, err := ioutil.ReadFile(path.Join(cgroupPath, "blkio.throttle.write_bps_device"))
	if err != nil {
		t.Fatalf("get write limit failed, %v", path.Join(cgroupPath, "blkio.throttle.write_bps_device"))
	}
	if !strings.Contains(string(writevalue), "20971520") {
		t.Fatalf("write limit set failed, the setted value is %s, target value is %v", string(writevalue), writeKbps)
	}

	readvalue, err := ioutil.ReadFile(path.Join(cgroupPath, "blkio.throttle.read_bps_device"))
	if err != nil {
		t.Fatalf("get read limit failed, %v", path.Join(cgroupPath, "blkio.throttle.read_bps_device"))
	}
	if !strings.Contains(string(readvalue), "10485760") {
		t.Fatalf("read limit set failed, the setted value is %s, target value is %v", string(readvalue), readKbps)
	}
}
