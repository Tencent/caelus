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

package util

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tencent/caelus/pkg/cadvisor"
	"github.com/tencent/caelus/pkg/nm-operator/hadoop"
	"github.com/tencent/caelus/pkg/nm-operator/types"
	global "github.com/tencent/caelus/pkg/types"

	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

// DiskUnit translates Gi to btye
const DiskUnit = int64(1024 * 1020 * 1024)

// InitCgroup set cgroup related environment
func InitCgroup(user, group, envCgroupPath string) error {
	executor := os.Getenv("CONTAINER_EXECUTOR")
	if executor != "org.apache.hadoop.yarn.server.nodemanager.LinuxContainerExecutor" {
		klog.Errorf("skip init cgroup, CONTAINER_EXECUTOR is %s", executor)
		return nil
	}
	// just handling limited cgroup sub systems
	subsys := []string{"cpu", "memory", "net_cls"}

	oldMask := syscall.Umask(0)
	defer syscall.Umask(oldMask)

	// get cgroup mount path from environment
	cgroupPath := hadoop.GetConfDataFromFile(hadoop.YarnSiteFile,
		"yarn.nodemanager.linux-container-executor.cgroups.mount-path")
	if envCgroupPath := os.Getenv(envCgroupPath); envCgroupPath != "" && envCgroupPath != cgroupPath {
		klog.Infof("change cgroup path from %s to %s", cgroupPath, envCgroupPath)
		cgroupPathSet := map[string]string{
			"yarn.nodemanager.linux-container-executor.cgroups.mount-path": envCgroupPath,
		}
		if err := hadoop.SetMultipleConfDataToFile(hadoop.YarnSiteFile, cgroupPathSet); err != nil {
			klog.Errorf("set cgroup cgroupPath err: %v", err)
			return err
		}
		cgroupPath = envCgroupPath
	}

	// get cgroup hierarchy path
	hierarchy := hadoop.GetConfDataFromFile(hadoop.YarnSiteFile,
		"yarn.nodemanager.linux-container-executor.cgroups.hierarchy")
	// create cgroup path and change file mode to 0750
	for _, sub := range subsys {
		hadoopPath := path.Join(cgroupPath, sub, hierarchy)
		if fileInfo, err := os.Stat(hadoopPath); os.IsNotExist(err) {
			if err = os.Mkdir(hadoopPath, 0750); err != nil {
				klog.Errorf("mkdir path %s err: %v", hadoopPath, err)
				return err
			}
			klog.Infof("create dir %s succ\n", hadoopPath)
		} else if err != nil {
			return err
		} else {
			if fileInfo.Mode() != 0750 {
				if err = os.Chmod(hadoopPath, 0750); err != nil {
					klog.Errorf("chmod path %s to 0750 err: %v", hadoopPath, err)
					return err
				}
				klog.Infof("change dir mode succ,%s, from %+v to 0750\n", hadoopPath, fileInfo.Mode())
			}
		}

		// change to admin users
		klog.Infof("chown cgroup for %s", hadoopPath)
		cmd := fmt.Sprintf("/bin/chown -R %s:%s %s", user, group, hadoopPath)
		out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		if err != nil {
			klog.Fatalf("chown cgroup path(%s) err: %v, output: %s", hadoopPath, err, string(out[:]))
		}
	}
	return nil
}

// GetContainersState return container info based on container ids given
func GetContainersState(cids []string, cManager cadvisor.Cadvisor) global.NMContainersState {
	cstats := global.NMContainersState{
		Cstats: make(map[string]*global.ContainerState),
	}

	for _, cid := range cids {
		cstats.Cstats[cid] = getConState(cid, cManager)
	}

	return cstats
}

// getConState return container state based on id in parameter
func getConState(cid string, cManager cadvisor.Cadvisor) *global.ContainerState {
	usage := &global.ContainerState{
		UsedCPU:   0,
		UsedMemMB: 0,
		CgPath:    "",
		PidList:   []int{},
	}
	cPath := filepath.Join(types.HadoopPath, cid)
	usage.CgPath = cPath
	// call cadvisor API to get cgroup level resource usage
	infos, err := cManager.ContainerInfoV2(cPath, cadvisorapiv2.RequestOptions{
		IdType:    cadvisorapiv2.TypeName,
		Count:     2,
		Recursive: false,
		MaxAge:    nil,
	})
	if err != nil {
		klog.V(4).Infof("collect container(%s) cpu usage err: %v", cPath, err)
		return usage
	}
	if len(infos) == 0 {
		klog.Errorf("collect container(%s) cpu usage is empty", cPath)
		return usage
	}

	info := infos[cPath]
	// calculate cpu consumption between the last two states
	if info.Spec.HasCpu {
		size := len(info.Stats)
		if size > 1 {
			total := info.Stats[size-1].Cpu.Usage.Total - info.Stats[size-2].Cpu.Usage.Total
			interval := info.Stats[size-1].Timestamp.Sub(info.Stats[size-2].Timestamp)
			usage.UsedCPU = float32(int(float64(total)*100/float64(interval.Nanoseconds())+0.5)) / 100
		}
	}
	if info.Spec.HasMemory {
		size := len(info.Stats)
		if size > 0 {
			usage.UsedMemMB = float32(info.Stats[size-1].Memory.WorkingSet) / float32(1024*1024)
		}
	}

	// using cgroup creation timestamp as container's start timestamp
	usage.StartTime = info.Spec.CreationTime

	return usage
}

// ExecuteYarnCMD execute yarn command
// The user parameter show who will execute the command
func ExecuteYarnCMD(cmds []string, user string) error {
	uidBytes, err := exec.Command("id", "-u", user).Output()
	if err != nil {
		klog.Errorf("get user id fail for %s: %v", user, err)
		return err
	}
	uidStr := strings.Trim(string(uidBytes), "\n")
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		klog.Errorf("translate string(%s) to integer err: %v", uidStr, err)
		return err
	}

	gidBytes, err := exec.Command("id", "-g", user).Output()
	if err != nil {
		klog.Errorf("get group id fail for %s: %v", user, err)
		return err
	}
	gidStr := strings.Trim(string(gidBytes), "\n")
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		klog.Errorf("translate string(%s) to integer err: %v", gidStr, err)
		return err
	}

	cmd := exec.Command("/bin/bash", cmds...)
	//  yarn command need the USER environment
	cmd.Env = append(syscall.Environ(), "USER="+user)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	out, err := cmd.Output()
	if err != nil {
		klog.Errorf("uid(%s) execute yarn cmd(%s) fail, output: %s, err:%v",
			uidStr, strings.Join(cmds, " "), string(out), err)
	}

	return err
}

// ExecuteYarnUpdateNodeResource execute update node resource
// The command description could be found from
// https://hadoop.apache.org/docs/stable/hadoop-yarn/hadoop-yarn-site/YarnCommands.html
// You should also add the dynamic-resources.xml on the ResourceManager machines,
// in case the backup switching and the resource will be reset
func ExecuteYarnUpdateNodeResource(yarnBinPath, user, mems, cores string) error {
	handler := func(cacheAddress bool) error {
		nmAddress := hadoop.GetNodeManagerAddress(cacheAddress)
		return ExecuteYarnCMD([]string{yarnBinPath, "rmadmin", "-updateNodeResource", nmAddress, mems, cores}, user)
	}

	// the NodeManager address may be changed dynamically, so try again if failed
	err := handler(true)
	if err != nil {
		klog.Errorf("execute yarn command err, try again: %v", err)
		err = handler(false)
	}

	return err
}

// WaitTimeout waiting for waitgroup with the specified max timeout.
// Return true if waiting timed out.
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

// GetContainerProcessGroupId get process group id for the container
func GetContainerProcessGroupId(cID string) int {
	// get container pid list
	cgPath := path.Join(types.CgroupRoot, "cpu", types.HadoopPath, cID)
	pidList, err := cgroups.GetPids(cgPath)
	if err != nil {
		klog.Errorf("collect container(%s) pid list err: %v", cgPath, err)
		return 0
	}
	if len(pidList) == 0 {
		klog.Errorf("list cgroup pid for %s, got nil", cID)
		return 0
	}

	// get process group id
	gpid, err := syscall.Getpgid(pidList[0])
	if err != nil {
		klog.Errorf("get group id for process id(%d) err: %v", pidList[0], err)
		// shall we return the original pid: pidList[0]?
		return 0
	}
	return gpid
}

// CheckPidAlive check if the pid is running by checking /proc/xxx
func CheckPidAlive(pid int) bool {
	pidProc := fmt.Sprintf("/proc/%d", pid)
	_, err := os.Stat(pidProc)
	if os.IsNotExist(err) {
		return false
	}

	return true
}

// GetDiskPartitionsName output all disk partitions name to caelus
func GetDiskPartitionsName() ([]string, error) {
	dirs := GetYarnLocalDirs()
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return []string{}, err
	}
	defer f.Close()
	diskPartitions := make(sets.String)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		for _, dir := range dirs {
			if strings.HasPrefix(dir, fields[1]) {
				diskPartitions = diskPartitions.Insert(strings.TrimSpace(fields[0]))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		klog.Infof("check /proc/self/mounts err: %v", err)
	}
	return diskPartitions.List(), nil
}

// GetYarnLocalDirs get yarn localdirs
func GetYarnLocalDirs() []string {
	localDirsStr := hadoop.GetYarnNodeManagerLocalDirs()
	return strings.Split(localDirsStr, ",")
}

// JudgePartitionCapForDataPath  if all partitions' size are too small, just set nodemanager as unhealthy by the disk health checker
// you should know that new value will override the original value for the disk health checker
func JudgePartitionCapForDataPath(diskMinCapGb int64, localDir string) error {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return err
	}
	defer f.Close()
	var mountPoint string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		if strings.HasPrefix(localDir, fields[1]) {
			mountPoint = strings.TrimSpace(fields[1])
			break
		}
	}
	if err := scanner.Err(); err != nil {
		klog.Infof("check /proc/self/mounts err: %v", err)
	}
	pStat, err := global.GetDiskPartitionStats(mountPoint)
	if err != nil {
		klog.Errorf("get partition state(%s) failed: %v", mountPoint, err)
		return err
	}
	// check if the partition size is too small, and giving a large size to make NM unhealthy
	if pStat.TotalSize < diskMinCapGb*DiskUnit {
		properties := make(map[string]string)
		properties["yarn.nodemanager.disk-health-checker.min-free-space-per-disk-mb"] = "10240000"
		err := hadoop.SetMultipleConfDataToFile(hadoop.YarnSiteFile, properties)
		if err != nil {
			return err
		}
	}
	return nil
}
