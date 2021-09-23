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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tencent/caelus/pkg/caelus/alarm"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var (
	// dataPartition is just for data path
	dataPartitionInHost      = "/data"
	dataPartitionInContainer = types.RootFS + "/data"

	dirRegexHost      = "^/data[1-9]$"
	dirRegexContainer = "^" + types.RootFS + "/data[1-9]$"

	diskSpaceCores *resource.Quantity
)

// DiskManager describe disk manager options for yarn
type DiskManager struct {
	types.YarnDisksConfig
	availablePartitions []PartitionInfo
	// just support yarn on k8s
	k8sClient kubernetes.Interface
	// cleaningLock is used to lock when cleaning disk space
	cleaningLock sync.Mutex
	// existTimestamp show the timestamp when pod is not ready
	existTimestamp *time.Time
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

// DiskPartitionStats show disk space size
type DiskPartitionStats struct {
	// TotalSize show total disk size in bytes
	TotalSize int64
	// UsedSize show used disk size in bytes
	UsedSize int64
	// FreeSize show free disk size in bytes
	FreeSize int64
}

// NewDiskManager create a new disk manager instance
func NewDiskManager(config types.YarnDisksConfig, k8sClient kubernetes.Interface) *DiskManager {
	return &DiskManager{
		YarnDisksConfig: config,
		availablePartitions: getAvailablePartitions(util.InHostNamespace, config.MultiDiskDisable,
			config.DiskMinCapacityGb),
		k8sClient: k8sClient,
	}
}

// Run start disk manager instance
func (d *DiskManager) Run(stopCh <-chan struct{}) {
	if !d.SpaceCheckEnabled {
		klog.V(2).Infof("disk space check not enabled for yarn")
		return
	}

	go wait.Until(d.checkDiskSpace, d.SpaceCheckPeriod.TimeDuration(), stopCh)
	klog.V(2).Infof("disk space check started for yarn")
}

// DiskSpaceToCores will check disk size, and output how many cores matching the size,
// there is a ratio between disk size and cores
func (d *DiskManager) DiskSpaceToCores() (*resource.Quantity, error) {
	if diskSpaceCores != nil {
		return diskSpaceCores, nil
	}

	var totalSize int64 = 0
	for _, p := range d.availablePartitions {
		klog.V(4).Infof("partition(%s) disk space state: %+v", p.MountPoint, p.TotalSize)
		totalSize += p.TotalSize
	}

	cores := int64(float64(totalSize)/float64(d.RatioToCore*types.DiskUnit) + 0.5)
	diskSpaceCores = resource.NewMilliQuantity(cores*types.CpuUnit, resource.DecimalSI)

	return diskSpaceCores, nil
}

// GetLocalDirs output yarn.nodemanager.local-dirs and yarn.nodemanager.log-dirs.
func (d *DiskManager) GetLocalDirs() (localDirs []string, logDirs []string) {
	// this may be waiting for a long time until full disk space has been released
	d.cleaningLock.Lock()
	defer d.cleaningLock.Unlock()

	for _, p := range d.availablePartitions {
		if _, err := os.Stat(p.LocalPath); os.IsNotExist(err) {
			klog.Warningf("local path(%s) not found, just creating", p.LocalPath)
			createLocalDir(p.LocalPath)
		}
		if _, err := os.Stat(p.LogPath); os.IsNotExist(err) {
			klog.Warningf("log path(%s) not found, just creating", p.LogPath)
			createLocalDir(p.LogPath)
		}

		localDirs = append(localDirs, p.LocalPath)
		logDirs = append(logDirs, p.LogPath)
	}

	return localDirs, logDirs
}

// checkDiskSpace check if the disk space is full, and may restart the offline pod and clear paths.
func (d *DiskManager) checkDiskSpace() {
	d.cleaningLock.Lock()
	defer d.cleaningLock.Unlock()

	restartNMPod, clearPaths := d.getClearPaths()
	if restartNMPod {
		err := d.restartNMPod()
		if err != nil {
			return
		}
		if len(clearPaths) != 0 {
			d.cleanDiskSpace(clearPaths)
		}
	}

	// if offline pod has exited for long time, we should clear disk space
	if d.OfflineExitedCleanDelay.TimeDuration() != 0 {
		pod, err := getOfflinePod()
		if err != nil {
			klog.Errorf("get offline pod failed when cleaning disk space: %v", err)
			return
		}
		if pod != nil {
			d.existTimestamp = nil
			return
		}
		if d.existTimestamp == nil {
			klog.Warningf("offline pod has exited, starting record timestamp")
			nowTime := time.Now()
			d.existTimestamp = &nowTime
			return
		}
		if d.existTimestamp.Add(d.OfflineExitedCleanDelay.TimeDuration()).Before(time.Now()) {
			msg := fmt.Sprintf("offline pod has exited for %v time, starting to clear disk space",
				d.OfflineExitedCleanDelay.TimeDuration())
			klog.Warningf(msg)
			//alarm.SendAlarm(msg)
			mountPoints := make(map[string]int64)
			for _, p := range d.availablePartitions {
				mountPoints[p.MountPoint] = 0
			}
			d.cleanDiskSpace(mountPoints)
		}
	}
}

// getClearPaths check if need to restart pod, and return paths which need to clear
func (d *DiskManager) getClearPaths() (restartNMPod bool, clearPaths map[string]int64) {
	restartNMPod = false
	clearPaths = make(map[string]int64)

	dataIsFull := false
	for _, p := range d.availablePartitions {
		pStat, err := getDiskPartitionStats(p.MountPoint)
		if err != nil {
			klog.Errorf("get partition state(%v) failed: %v", p, err)
			continue
		}

		freeGb := pStat.FreeSize / types.DiskUnit

		if freeGb < d.SpaceCheckReservedGb || pStat.FreeSize < int64(d.SpaceCheckReservedPercent*float64(pStat.TotalSize)) {
			clearPaths[p.MountPoint] = freeGb
			if dirIsDataPath(p.MountPoint) {
				dataIsFull = true
			}
		}

	}
	// no paths need to clear, just return
	if len(clearPaths) == 0 {
		return
	}

	msg := fmt.Sprintf("paths(%+v) has little disk space, and ", clearPaths)
	handleMsg := "disk space cleaning is false, nothing to do"
	if !d.SpaceCleanDisable {
		restartNMPod = true
		handleMsg = "disk space cleaning is true, will restart offline job and clear disk space"
		if d.SpaceCleanJustData {
			// just restart pod to release /data path, no need to clear others partitions disk space
			clearPaths = make(map[string]int64)
			if dataIsFull {
				handleMsg = "disk space cleaning is true and just clean /data path, now data path is full, will restart offline pod"
			} else {
				restartNMPod = false
				handleMsg = "disk space cleaning is true and just clean /data path, now data path is not full, nothing to do"
			}
		}
	}

	// send alarm
	alarm.SendAlarm(msg + handleMsg)
	klog.V(2).Infof(msg + handleMsg)

	return restartNMPod, clearPaths
}

// restartOfflinePod will restart nodemanager pod
func (d *DiskManager) restartNMPod() error {
	// kill offline pod
	pod, err := getOfflinePod()
	if err != nil {
		klog.Errorf("get offline pod failed when cleaning disk space: %v", err)
		return err
	}
	if pod == nil {
		klog.Errorf("get no offline pod when cleaning disk space")
		return fmt.Errorf("no offline pod found")
	}
	klog.V(2).Infof("starting to kill offline nodemanager pod: %s-%s", pod.Namespace, pod.Name)
	options := metav1.NewDeleteOptions(0)
	err = d.k8sClient.CoreV1().Pods(pod.Namespace).Delete(pod.Name, options)
	if err != nil {
		klog.Errorf("kill offline pod(%s-%s) failed when cleaning disk space: %v", pod.Namespace, pod.Name, err)
		return err
	}
	klog.V(2).Infof("offline nodemanager pod(%s-%s) deleted success ", pod.Namespace, pod.Name)

	// waiting until new offline pod is ready, it may generate deleted files when cleaning disk space during pod deleting,
	// so we just waiting until new pod is ready.
	err = wait.PollImmediateInfinite(time.Duration(1*time.Second), func() (bool, error) {
		klog.V(2).Infof("waiting new offline pod when cleaning disk space")
		newPod, err := getOfflinePod()
		if err != nil {
			return false, err
		}
		if newPod == nil {
			return false, nil
		}
		// Pending is the temporary phase which may not be catch, so just check Running phase
		if newPod.Name != pod.Name && newPod.Status.Phase == "Running" {
			klog.V(2).Infof("new offline nodemanager pod is running: %s-%s", newPod.Namespace, newPod.Name)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		klog.Errorf("waiting new offline nodemanager pod running failed: %v", err)
		return err
	}

	return nil
}

// cleanDiskSpace clear disk space based on given paths
func (d *DiskManager) cleanDiskSpace(fullMountPoints map[string]int64) {
	defer func(start time.Time) {
		klog.V(2).Infof("cleaning disk space cost: %s", time.Now().Sub(start))
	}(time.Now())

	// clear disk space
	for dir := range fullMountPoints {
		if dirIsDataPath(dir) {
			klog.V(2).Infof("/data path is full, no need to clear disk space, restart pod is ok")
			// /data path is mounted in emptydir for nodemanager pod, restart pod will release the space
			continue
		}

		localDir := path.Join(dir, subLocalDir)
		klog.V(2).Infof("starting to clear path: %s", localDir)
		if err := clearDir(localDir); err != nil {
			klog.Errorf("cleaning path(%s) failed: %v", localDir, err)
		}
		logDir := path.Join(dir, subLogDir)
		klog.V(2).Infof("starting to clear path: %s", logDir)
		if err := clearDir(logDir); err != nil {
			klog.Errorf("cleaning path(%s) failed: %v", logDir, err)
		}
	}
}

// getAvailablePartitions get available partitions for nodemanager
func getAvailablePartitions(hostNamespace bool, multiDiskDisable bool, diskMinCapGb int64) []PartitionInfo {
	partInfos := []PartitionInfo{}
	if !multiDiskDisable {
		// get extra multi disks info without /data
		// if the nodemanager need multi disks, the pod should mount host path "/" to container as "/rootfs"
		partInfos = probeLocalAndLogDirs(hostNamespace, diskMinCapGb)
		klog.V(2).Infof("multi disks enabled for yarn, available partitions: %+v", partInfos)
	} else {
		klog.V(2).Infof("multi disks disabled for yarn, will use /data path")
	}
	if len(partInfos) == 0 {
		klog.V(2).Infof("extra partitions disk is empty, adding /data info")
		dataPartInfo, err := generatePartitionForDataPath(hostNamespace)
		if err != nil {
			klog.Fatalf("generate /data partition info failed: %v", err)
		}
		partInfos = append(partInfos, dataPartInfo)
	}

	return partInfos
}

// probeLocalAndLogDirs check extra paths from /data1....., not containing /data
func probeLocalAndLogDirs(hostNamespace bool, diskSize int64) (partInfos []PartitionInfo) {
	dirWithRegx := dirRegexHost
	if !hostNamespace {
		dirWithRegx = dirRegexContainer
	}

	partitions, err := getDiskPartitions(dirWithRegx)
	if err != nil {
		klog.Fatalf("get mount point fail with pattern(%s): %v", dirWithRegx, err)
		return
	}

	if len(partitions) == 0 {
		klog.Warningf("no extra disk paths found!")
		return
	}

	for _, partition := range partitions {
		stats, err := getDiskPartitionStats(partition)
		if err != nil {
			klog.Errorf("failed to get %s stats: %v", partition, err)
			continue
		}
		// select the mount point which is big enough.
		if stats.TotalSize < diskSize*types.DiskUnit {
			klog.Warningf("path(%s) has little disk space(%d), just ignore", partition, stats.TotalSize)
			continue
		}

		// create local data directory
		localDir := path.Join(partition, subLocalDir)
		if err := createLocalDir(localDir); err != nil {
			continue
		}

		// create log directory
		logDir := path.Join(partition, subLogDir)
		if err := createLocalDir(logDir); err != nil {
			continue
		}

		partInfos = append(partInfos, PartitionInfo{
			MountPoint: partition,
			LocalPath:  localDir,
			LogPath:    logDir,
			TotalSize:  stats.TotalSize,
		})
	}

	return partInfos
}

// generatePartitionForDataPath generate partition info for the /data path,
// there is something special for the /data, such as disk space size and local directory for nodemanager
func generatePartitionForDataPath(hostNamespace bool) (PartitionInfo, error) {
	dataMountPoint := dataPartitionInHost
	if !hostNamespace {
		dataMountPoint = dataPartitionInContainer
	}

	pStat, err := getDiskPartitionStats(dataMountPoint)
	if err != nil {
		klog.Errorf("get /data partition state(%s) failed: %v", dataMountPoint, err)
		return PartitionInfo{}, err
	}

	// /data has been mounted in emptydir type, so just set /data/yarnenv/local and /data/yarnenv/container-logs.
	dataPartition := "/data"
	localPath := path.Join(dataPartition, subLocalDir)
	logPath := path.Join(dataPartition, subLogDir)

	// for /data path, online jobs will also use, so we just give half the space to offline jobs.
	// maybe freeSize is better, while this may be always changing.
	totalSize := pStat.TotalSize / 2

	return PartitionInfo{
		MountPoint: dataMountPoint,
		TotalSize:  totalSize,
		LocalPath:  localPath,
		LogPath:    logPath,
	}, nil
}

// getDiskPartitions output all mounted partitions based on regexPattern
func getDiskPartitions(regexPattern string) ([]string, error) {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return []string{}, err
	}
	defer f.Close()

	regex := regexp.MustCompile(regexPattern)

	var mPoints []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		mp := strings.TrimSpace(fields[1])
		if regex.MatchString(mp) {
			mPoints = append(mPoints, mp)
		}

	}
	if err := scanner.Err(); err != nil {
		klog.Infof("check /proc/self/mounts err: %v", err)
	}
	return mPoints, nil
}

// getDiskPartitionStats output disk space stats for the partition
func getDiskPartitionStats(partitionName string) (*DiskPartitionStats, error) {
	stat := syscall.Statfs_t{}

	err := syscall.Statfs(partitionName, &stat)
	if err != nil {
		return nil, err
	}

	dStats := &DiskPartitionStats{
		TotalSize: int64(stat.Blocks) * stat.Bsize,
		FreeSize:  int64(stat.Bfree) * stat.Bsize,
	}
	dStats.UsedSize = dStats.TotalSize - dStats.FreeSize

	return dStats, nil
}

// createLocalDir create local data and log directory
func createLocalDir(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0777); err != nil {
		if !os.IsExist(err) {
			klog.Errorf("create directory %s failed, %v", dirPath, err)
			return err
		}
	}
	if err := os.Chmod(dirPath, 0777); err != nil {
		klog.Errorf("assign 777 to path(%s) err: %v", dirPath, err)
		return err
	}

	return nil
}

// dirIsDataPath check if the directory is the /data path
func dirIsDataPath(dir string) bool {
	if dir == dataPartitionInHost || dir == dataPartitionInContainer {
		return true
	}

	return false
}

// clearDir release disk space based on given directory
func clearDir(dir string) error {
	// Remove is dangerous! should check again
	if !strings.HasSuffix(dir, subLocalDir) && !strings.HasSuffix(dir, subLogDir) {
		klog.Warningf("path(%s) is not nodemanager path, do not clear", dir)
		return nil
	}

	subDirs, err := ioutil.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, info := range subDirs {
		fullPath := filepath.Join(dir, info.Name())
		err = os.RemoveAll(fullPath)
		if err != nil {
			// just warning, no need to return error
			klog.Errorf("remove path(%s) err: %v", fullPath, err)
		} else {
			klog.V(4).Infof("remove path(%s) success", fullPath)
		}
	}
	klog.V(2).Infof("path clean finished: %s", dir)

	return nil
}
