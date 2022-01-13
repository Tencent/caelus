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

package yarn

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tencent/caelus/pkg/caelus/alarm"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	global "github.com/tencent/caelus/pkg/types"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var (
	diskSpaceCores *resource.Quantity

	// DiskSpaceLimited is the temporary code for old NM image, and will drop in feature
	DiskSpaceLimited bool
)

// DiskManager describe disk manager options for yarn
type DiskManager struct {
	types.YarnDisksConfig
	availablePartitions []PartitionInfo
	// just support yarn on k8s
	k8sClient kubernetes.Interface
	// existTimestamp show the timestamp when pod is not ready
	existTimestamp *time.Time
	// ginit group the options for contracting with nodemanager
	ginit GInitInterface
	// firstCheck means firstly to check disks
	firstCheck bool
}

// PartitionInfo describe directory info on partition
type PartitionInfo struct {
	MountPoint string
	// LocalPath local data path for nodemanager on this partition
	LocalPath string
	// LogPath log path for nodemanager on this partition
	LogPath   string
	TotalSize int64
}

// NewDiskManager create a new disk manager instance
func NewDiskManager(config types.YarnDisksConfig, k8sClient kubernetes.Interface, ginit GInitInterface) *DiskManager {
	d := &DiskManager{
		YarnDisksConfig: config,
		k8sClient:       k8sClient,
		ginit:           ginit,
	}
	d.fillAvailablePartitions()

	return d
}

// Run start disk manager instance
func (d *DiskManager) Run(stopCh <-chan struct{}) {
	if d.DisableScheduler {
		go wait.Until(d.checkDiskSpace, d.SpaceCheckPeriod.TimeDuration(), stopCh)
		klog.V(2).Infof("disk space check started for yarn")
	} else {
		klog.V(2).Infof("disk space check disabled for yarn")
	}
}

// DiskSpaceToCores will check disk size, and output how many cores matching the size,
// there is a ratio between disk size and cores
func (d *DiskManager) DiskSpaceToCores() (*resource.Quantity, error) {
	if diskSpaceCores != nil {
		return diskSpaceCores, nil
	}
	if len(d.availablePartitions) == 0 {
		// the NM pod may be just started, we need to check again
		d.fillAvailablePartitions()
		if len(d.availablePartitions) == 0 {
			return nil, fmt.Errorf("not found available partitions")
		}
	}

	var totalSize int64
	for _, p := range d.availablePartitions {
		klog.V(4).Infof("partition(%s) disk space state: %+v", p.MountPoint, p.TotalSize)
		totalSize += p.TotalSize
	}

	cores := int64(float64(totalSize)/float64(*d.RatioToCore*types.DiskUnit) + 0.5)
	diskSpaceCores = resource.NewMilliQuantity(cores*types.CPUUnit, resource.DecimalSI)

	return diskSpaceCores, nil
}

// checkDiskSpace check if the disk space and wil disable schedule if the space is nearly full.
func (d *DiskManager) checkDiskSpace() {
	var scheduleDisabled bool
	var keys = []string{
		"yarn.nodemanager.disk-health-checker.min-free-space-per-disk-mb",
		"yarn.nodemanager.disk-health-checker.max-disk-utilization-per-disk-percentage",
		"yarn.nodemanager.disk-health-checker.min-healthy-disks",
	}
	values, err := d.ginit.GetProperty(YarnSite, keys, false)
	if err != nil {
		klog.Errorf("request disk health checker failed err: %v", err)
		return
	}
	minFreeMb, err := strconv.Atoi(values["yarn.nodemanager.disk-health-checker.min-free-space-per-disk-mb"])
	if err != nil {
		klog.Errorf("invalid yarn.nodemanager.disk-health-checker.min-free-space-per-disk-mb: %s",
			values["yarn.nodemanager.disk-health-checker.min-free-space-per-disk-mb"])
		return
	}
	maxUsedPer, err := strconv.Atoi(values["yarn.nodemanager.disk-health-checker.max-disk-utilization-per-disk-percentage"])
	if err != nil {
		klog.Errorf("invalid yarn.nodemanager.disk-health-checker.max-disk-utilization-per-disk-percentage: %s",
			values["yarn.nodemanager.disk-health-checker.max-disk-utilization-per-disk-percentage"])
		return
	}
	minHealthyPer, err := strconv.ParseFloat(values["yarn.nodemanager.disk-health-checker.min-healthy-disks"], 64)
	if err != nil {
		klog.Errorf("invalid yarn.nodemanager.disk-health-checker.min-healthy-disks: %s",
			values["yarn.nodemanager.disk-health-checker.min-healthy-disks"])
		return
	}

	if len(d.availablePartitions) == 0 {
		// the NM pod may be just started, we need to check again
		d.fillAvailablePartitions()
		if len(d.availablePartitions) == 0 {
			klog.Errorf("disk partitions info is empty:%v", err)
			return
		}
	}

	healthyDiskNum := 0
	for _, p := range d.availablePartitions {
		pStat, err := global.GetDiskPartitionStats(p.MountPoint)
		if err != nil {
			klog.Errorf("get partition state(%v) failed: %v", p, err)
			continue
		}
		// add buffer size and take action ahead the NM capacity checking
		if (pStat.FreeSize-d.DiskCapConservativeValue*types.DiskUnit) <= int64(minFreeMb)*types.MemUnit ||
			float64(pStat.UsedSize+d.DiskCapConservativeValue*types.DiskUnit)/float64(pStat.TotalSize) >=
				float64(maxUsedPer/100) {
			continue
		}
		healthyDiskNum++
	}
	if healthyDiskNum == 0 || (len(d.availablePartitions) > 1 &&
		float64(healthyDiskNum-d.DiskNumConservativeValue)/float64(len(d.availablePartitions)) <= minHealthyPer) {
		if !scheduleDisabled {
			alarm.SendAlarm("schedule is closing, the reason is that disks is full")
			klog.V(2).Infof("schedule is closing, the reason is that disks is full")
			err := d.ginit.DisableSchedule()
			if err != nil {
				klog.Errorf("disable yarn schedule err: %v", err)
				return
			}
			scheduleDisabled = true
			metrics.NodeScheduleDisabled(1)
		}
	} else {
		// if the process restarted, the firstCheck will make sure to recover the schedule state
		if d.firstCheck || scheduleDisabled {
			err := d.ginit.EnableSchedule()
			if err != nil {
				klog.Errorf("enable yarn schedule err: %v", err)
				return
			}
			scheduleDisabled = false
			d.firstCheck = false
			metrics.NodeScheduleDisabled(0)
		}
	}
}
func (d *DiskManager) fillAvailablePartitions() {
	diskPartitions, err := d.ginit.GetDiskPartitions()
	if err != nil {
		klog.Errorf("get disk partitions info failed:%v", err)
		if strings.Contains(err.Error(), "404 Not Found") {
			// temporary codes for old NM image, and will drop in feature
			mountPoint := "/data"
			if !util.InHostNamespace {
				mountPoint = path.Join(types.RootFS, "/data")
			}
			stats, err := global.GetDiskPartitionStats(mountPoint)
			if err != nil {
				klog.Errorf("failed to get %s stats: %v", mountPoint, err)
			} else {
				d.availablePartitions = []PartitionInfo{{
					MountPoint: mountPoint,
					TotalSize:  stats.TotalSize,
				}}
				// just the fixed value
				if stats.TotalSize <= 50*types.DiskUnit {
					DiskSpaceLimited = true
				}
			}
		}
		return
	}
	klog.V(4).Infof("disk partitions is %v", diskPartitions)
	d.availablePartitions = getPartitionsInfos(diskPartitions)
}

// getPartitionsInfos get mount point and total size for the partitions
func getPartitionsInfos(diskPartitions []string) []PartitionInfo {
	var partInfos []PartitionInfo
	if len(diskPartitions) == 0 {
		return partInfos
	}

	partitions, err := getMountPoints(diskPartitions)
	if err != nil {
		klog.Fatalf("get mount point failed: %v", err)
		return nil
	}
	// get total size for partitions
	for _, partition := range partitions {
		stats, err := global.GetDiskPartitionStats(partition)
		if err != nil {
			klog.Errorf("failed to get %s stats: %v", partition, err)
			continue
		}
		partInfos = append(partInfos, PartitionInfo{
			MountPoint: partition,
			TotalSize:  stats.TotalSize,
		})
		klog.V(2).Infof("disk partition %s with total size: %d", partition, stats.TotalSize)
	}
	return partInfos
}

// getMountPoints output all mounted partitions based on regexPattern
func getMountPoints(diskPartitions []string) ([]string, error) {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return []string{}, err
	}
	defer f.Close()

	var mPoints []string
	scanner := bufio.NewScanner(f)
	for _, diskPartition := range diskPartitions {
		for scanner.Scan() {
			txt := scanner.Text()
			fields := strings.Split(txt, " ")
			dp := strings.TrimSpace(fields[0])
			// If it matches the partition, output the first mount point and exit, otherwise
			// the mount point will be calculated repeatedly
			if dp == diskPartition {
				mPoints = append(mPoints, fields[1])
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		klog.Infof("check /proc/self/mounts err: %v", err)
	}
	return mPoints, nil
}
