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

	"k8s.io/klog"
)

/*
	The package is used to support block io, including read and write
*/

const (
	BlkioSubsystem    = "blkio"
	blockDir          = "/sys/block"
	ReadThrottleFile  = "blkio.throttle.read_bps_device"
	WriteThrottleFile = "blkio.throttle.write_bps_device"

	// only supports cfq scheduler(cat /sys/block/sda/queue/scheduler)
	weightDevice = "blkio.weight_device"

	offlineWeight = 10
)

// BlkioLimitSet set blkio readKiBps, writeKiBps limit of given cgroup path
func BlkioLimitSet(pathInSubSystem string, writeKiBps, readKiBps map[string]uint64) {
	root := GetRoot()

	// replace disk name to major&minor
	sdx := make(map[string]string)

	writeKiBpsNew := make(map[string]uint64)
	for disk, rate := range writeKiBps {
		deviceNumber, err := getBlockDeviceNumbers(disk)
		if err != nil {
			klog.Errorf("Get major and minor failed for %s, err:%v", disk, err)
			continue
		}
		writeKiBpsNew[deviceNumber] = rate
		// record the mapping relation
		sdx[disk] = deviceNumber
	}

	readKiBpsNew := make(map[string]uint64)
	for disk, rate := range readKiBps {
		if _, ok := sdx[disk]; ok {
			readKiBpsNew[sdx[disk]] = rate
			continue
		}
		deviceNumber, err := getBlockDeviceNumbers(disk)
		if err != nil {
			klog.Errorf("Get major and minor failed for %s, err:%v", disk, err)
			continue
		}
		readKiBpsNew[deviceNumber] = rate
	}

	cgroupPath := path.Join(root, "blkio", pathInSubSystem)
	klog.V(4).Infof("Setting blkio limit for path:%v, write limit:%v, read limit:%v, the disks:%v",
		cgroupPath, writeKiBps, readKiBps, sdx)

	//Set write limit
	writeLimitSet(cgroupPath, writeKiBpsNew)
	//Set read limit
	readLimitSet(cgroupPath, readKiBpsNew)

	return
}

func writeLimitSet(cgroupPath string, limits map[string]uint64) {
	for disk, kiBps := range limits {
		// the setting is not available for buffer write, while available for direct write
		value := disk + " " + strconv.FormatUint(kiBps<<10, 10)
		err := WriteFile([]byte(value), cgroupPath, WriteThrottleFile)
		if err != nil {
			klog.Errorf("Default kernel Set write limit faild for %s, err:%v", disk, err)
		}
	}
	return
}

func readLimitSet(cgroupPath string, limits map[string]uint64) {
	for disk, kiBps := range limits {
		value := disk + " " + strconv.FormatUint(kiBps<<10, 10)
		err := WriteFile([]byte(value), cgroupPath, ReadThrottleFile)
		if err != nil {
			klog.Errorf("Set read limit faild for %s, err:%v", disk, err)
			continue
		}
	}
	return
}

func getBlockDeviceNumbers(name string) (string, error) {
	dev, err := ioutil.ReadFile(path.Join(blockDir, name, "/dev"))
	if err != nil {
		return "", err
	}
	return strings.Replace(string(dev), "\n", "", -1), nil
}

// SetBlkioWeight sets blkio weight for the given deviceNames and cgroups
func SetBlkioWeight(pathInSubSystem string, deviceNames []string) error {
	root := GetRoot()
	cgroupPath := path.Join(root, "blkio", pathInSubSystem)
	// unset all read/write bps configure, otherwise writting to weight file will fail with "Device or resource busy"
	for _, file := range []string{ReadThrottleFile, WriteThrottleFile} {
		data, err := ioutil.ReadFile(path.Join(cgroupPath, file))
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.Split(line, " ")
			if len(parts) != 2 {
				continue
			}
			value := parts[0] + " 0"
			if err := WriteFile([]byte(value), cgroupPath, file); err != nil {
				return fmt.Errorf("set write limit %q for %s: %v", value, file, err)
			}
		}
	}

	weightFile := weightDevice
	// set weight
	for _, deviceName := range deviceNames {
		deviceNumber, err := getBlockDeviceNumbers(deviceName)
		if err != nil {
			klog.Errorf("Get major and minor for %s: %v", deviceName, err)
			continue
		}
		value := fmt.Sprintf("%s %d", deviceNumber, offlineWeight)
		if err := WriteFile([]byte(value), cgroupPath, weightFile); err != nil {
			err1 := fmt.Errorf("set %q for %s: %v", value, weightFile, err)
			// check if scheduler is cfq, e.g.
			// cat /sys/block/sda/queue/scheduler
			// noop deadline [cfq]
			schedulerFile := fmt.Sprintf("/sys/block/%s/queue/scheduler", deviceName)
			data, err := ioutil.ReadFile(schedulerFile)
			if err != nil {
				klog.Warningf("%v, also read file %s: %v", err1, schedulerFile, err)
				continue
			}
			if !strings.Contains(string(data), "[cfq]") {
				// set weight_device for a none cfq disk may return -bash: echo: write error: Operation not supported
				klog.Warningf("%v, this happens because blkio cgroup is lack of %s and device "+
					"scheduler for %s %q is not cfq", err1, weightDevice, deviceName, string(data))
				continue
			}
			klog.Warning(err1)
		}
		klog.V(4).Infof("set %s to %s for %s", path.Join(cgroupPath, weightFile), value, deviceName)
	}
	return nil
}
