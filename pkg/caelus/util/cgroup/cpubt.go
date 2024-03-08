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
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/shirou/gopsutil/cpu"
	"k8s.io/klog/v2"
)

/*
	The package is used to support cpu offline, just support for tencent os
*/

const (
	offlineAttribute = "cpu.offline"
	offlinePath      = "/proc/offline/"
)

var (
	// cpuBTSupported is the variables to show if bt is supported
	cpuBTSupported = false
	cpuBTChecked   = false
	// cpuTotal is the total numbers of cores on machine
	cpuTotal = 0
)

// CPUOfflineSupported check if offline feature is supported
func CPUOfflineSupported() bool {
	if cpuBTChecked {
		return cpuBTSupported
	}

	root := GetRoot()
	_, err := os.Stat(path.Join(root, CPUSubsystem, offlineAttribute))
	if err != nil {
		klog.Errorf("checking BT file(%s) err: %v", offlineAttribute, err)
		cpuBTSupported = false
	} else {
		cpuBTSupported = true
	}

	cpuBTChecked = true
	return cpuBTSupported
}

// CPUOfflineSet enable /sys/fs/cgroup/cpu,cpuacct/xx/cpu.offline
func CPUOfflineSet(pathInSubSystem string, enable bool) error {
	value := "1"
	if !enable {
		value = "0"
	}
	root := GetRoot()
	cgroupPath := path.Join(root, "cpu,cpuacct", pathInSubSystem)
	klog.V(4).Infof("Starting to set cpu offline for path:%v", cgroupPath)

	err := WriteFile([]byte(value), cgroupPath, offlineAttribute)
	if err != nil && !os.IsNotExist(err) {
		klog.Errorf("Open cpu offline feature err: %v", err)
	}

	return nil
}

// CPUChildOfflineSet set offline feature for children cgroups
func CPUChildOfflineSet(pathInSubSystem string, enable bool) error {
	value := "1"
	if !enable {
		value = "0"
	}
	root := GetRoot()
	cgPath := path.Join(root, "cpu,cpuacct", pathInSubSystem)
	klog.V(4).Infof("Starting to set child cpu offline for path:%v", cgPath)

	err := filepath.Walk(cgPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			klog.Errorf("walk path(%s) err: %v", cgPath, err)
			return err
		}
		if f.IsDir() {
			err = WriteFile([]byte(value), path, offlineAttribute)
			if err != nil {
				klog.Errorf("set cgroup(%s) offline feature err: %v", path, err)
			}
		}
		return err
	})
	if err != nil {
		klog.Errorf("looping set cgroup(%s) offline feature err: %v", cgPath, err)
	}

	return err
}

// CPUOfflineLimit limit offline task running on limited cores
func CPUOfflineLimit(cpuLimitNum int, minPercent int) error {
	if cpuTotal == 0 {
		cpuInfo, err := cpu.Info()
		if err != nil {
			klog.Errorf("get machine cpu info err: %v", err)
			return err
		}
		cpuTotal = len(cpuInfo)
	}

	if cpuLimitNum > cpuTotal {
		klog.Errorf("limits cpus(%d) is more than total cpus(%d)", cpuLimitNum, cpuTotal)
		return fmt.Errorf("cpu limit number too big")
	}
	percent := (cpuLimitNum * 100) / cpuTotal
	if percent < minPercent {
		klog.Warningf("offline limit is too small(%d), reset to %d", percent, minPercent)
		percent = minPercent
	}
	klog.V(4).Infof("set offline limit(limits %d, total %d): %d", cpuLimitNum, cpuTotal, percent)

	cpus, err := ioutil.ReadDir(offlinePath)
	if err != nil {
		klog.Errorf("read dir /proc/offline err: %v", err)
		return err
	}
	for _, f := range cpus {
		p := path.Join(offlinePath, f.Name())
		// the last char will be trimed, so we added the space char.
		value := fmt.Sprintf("%d ", percent)
		err = ioutil.WriteFile(p, []byte(value), 0664)
		if err != nil {
			klog.Errorf("write offline file(%s) err: %v", p, err)
		}
	}

	return nil
}

// CPUOfflineUsage get cpu usage for offline cgroup
func CPUOfflineUsage(cgroupPath string) (uint64, error) {
	root := GetRoot()
	usageFile := path.Join(root, "cpu,cpuacct", cgroupPath, "cpuacct.bt_usage")
	usageBytes, err := ioutil.ReadFile(usageFile)
	if err != nil {
		klog.Errorf("read offline usage file(%s) err: %v", usageFile, err)
		return 0, err
	}

	usage, err := strconv.Atoi(string(usageBytes))
	if err != nil {
		klog.Errorf("invalid usage(%s) err: %v", string(usageBytes), err)
		return 0, err
	}

	return uint64(usage), nil
}

// CPUOfflineSetShares set value to /sys/fs/cgroup/cpu,cpuacct/xx/cpu.bt_shares
func CPUOfflineSetShares(pathInSubSystem string, value string) error {
	root := GetRoot()
	cgroupPath := path.Join(root, "cpu,cpuacct", pathInSubSystem)
	klog.V(4).Infof("Starting to set cpu bt shares for path:%v", cgroupPath)

	err := WriteFile([]byte(value), cgroupPath, "cpu.bt_shares")
	if err != nil {
		klog.Errorf("set cpu bt shares err:%v", err)
	}

	return nil
}
