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

package online

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	// -1 means has not starting collect pid number
	defaultPidNumber = -1
)

// Interface describes online jobs interfaces
type Interface interface {
	Name() string
	Run(stop <-chan struct{})

	GetOnlineCgroups() []string
}

// onlineManager describes options for online job manager
type onlineManager struct {
	types.OnlineConfig
	jobSpecs map[string]*jobSpec
	stStore  statestore.StateStore
}

// jobSpec group online jobs information
type jobSpec struct {
	regexp *regexp.Regexp
	// one regexp may map multi cgroup paths, such as osd_1, osd_2
	cgroups map[string]*cgroupSpec
}

// cgroupSpec describes cgroup related information
type cgroupSpec struct {
	// if the cgroup is fixed, such as managed by system service
	// the pid list are put into the cgroup automatically.
	fixed bool
	// pid list number for the cgroup
	pidNum int
}

// NewOnlineManager news an online manager instance
func NewOnlineManager(config types.OnlineConfig, stSotre statestore.StateStore) Interface {
	online := &onlineManager{
		OnlineConfig: config,
		stStore:      stSotre,
		jobSpecs:     make(map[string]*jobSpec),
	}

	// generate cgroup spec
	for _, job := range config.Jobs {
		klog.V(4).Infof("generating regexp for job(%s:%s)", job.Name, job.Command)
		exp, err := regexp.Compile(job.Command)
		if err != nil {
			klog.Fatalf("invalid regexp expression(%s): %v", job.Command, err)
		}

		online.jobSpecs[job.Name] = &jobSpec{
			regexp:  exp,
			cgroups: make(map[string]*cgroupSpec),
		}
	}

	return online
}

// Name returns module name
func (o *onlineManager) Name() string {
	return "ModuleOnline"
}

// Run main loop
func (o *onlineManager) Run(stop <-chan struct{}) {
	pidCg := o.PidToCgroup
	go func() {
		// should call before loop, generating cgroup paths ahead
		o.generateOnlineCgroups()
		klog.V(2).Info(jobSpecsStr(o.jobSpecs))

		// check online job pid list
		if pidCg.PidCheckInterval.Seconds() != 0 {
			go wait.Forever(func() {
				klog.V(4).Infof("check online job pids periodically")
				o.generateOnlineCgroups()
			}, pidCg.PidCheckInterval.TimeDuration())
		} else {
			klog.Warning("online job pid check periodically is disabled")
		}

		// check online jobs cgroup
		go wait.Forever(func() {
			klog.V(4).Infof("check online job cgroups periodically")
			changed := o.checkOnlineCgroupPath()
			if changed {
				klog.Info("online job cgroup pids changed, need generate cgroups again")
				o.generateOnlineCgroups()
			}
		}, pidCg.CgroupCheckInterval.TimeDuration())

		klog.V(2).Infof("online manager started successfully")
	}()
}

// GetOnlineCgroups get online jobs cgroup paths
func (o *onlineManager) GetOnlineCgroups() []string {
	paths := sets.NewString()
	for _, p := range o.jobSpecs {
		for k := range p.cgroups {
			paths.Insert(k)
		}
	}
	return paths.List()
}

/*
generateOnlineCgroups moves online pids to specific cgroup path,
if the pid is system service, then using system cgroup path directly.
it the online pid is docker service, the function has not covered.

11:net_cls:/
10:hugetlb:/
9:perf_event:/
8:blkio:/system.slice/system-ceph\x2dosd.slice
7:memory:/system.slice/system-ceph\x2dosd.slice
6:cpuset:/
5:devices:/system.slice/system-ceph\x2dosd.slice
4:cpu,cpuacct:/system.slice/system-ceph\x2dosd.slice
3:freezer:/
2:pids:/system.slice/system-ceph\x2dosd.slice

The system service does not manage cpuset cgroup subystem, so we could get all pids from cpuset root path,
and move to specific cgroup.
*/
func (o *onlineManager) generateOnlineCgroups() {
	allPidList := getMachinePidList()

	updated := false
	for jobName, spec := range o.jobSpecs {
		// get related pid list for the job
		matchedPidList := o.matchJobPidList(allPidList, jobName)
		if len(matchedPidList) == 0 {
			klog.Warningf("no pids found for online job: %s", jobName)
			continue
		}

		// update cgroup spec
		updated = updated || updateJobSpec(matchedPidList, jobName, spec)
	}
	if updated {
		o.notifyContainerManager()
	}

	klog.V(3).Info(jobSpecsStr(o.jobSpecs))
}

// notifyContainerManager notify cadvisor to collect new cgroups
func (o *onlineManager) notifyContainerManager() {
	var newCgs []string
	for _, spec := range o.jobSpecs {
		for cg := range spec.cgroups {
			if strings.HasPrefix(cg, types.CgroupNonK8sOnline) {
				cg = types.CgroupNonK8sOnline
			}
			newCgs = append(newCgs, cg)
		}
	}

	klog.Infof("notify container stat with new cgroups: %v", newCgs)
	o.stStore.AddExtraCgroups(newCgs)
}

// checkOnlineCgroupPath check if the cgroup pid number has changed,
// if changed, indicating that online job pid changed, need to collect again.
func (o *onlineManager) checkOnlineCgroupPath() bool {
	changed := false
	for jobName, spec := range o.jobSpecs {
		for cg, cgSpec := range spec.cgroups {
			if cgSpec.fixed {
				continue
			}
			fullPath := cgroup.GetMemoryCgroupPath(cg)
			pids, err := cgroup.GetPids(fullPath)
			if err != nil {
				klog.Errorf("get pids from cgroup path(%s) err: %v", fullPath, err)
				continue
			}
			newNum := len(pids)

			// just check pid number, better to check pid list
			if newNum != cgSpec.pidNum {
				klog.V(2).Infof("online job(%s) cgroup pid list changed, from %d to %d",
					jobName, cgSpec.pidNum, newNum)
				cgSpec.pidNum = newNum
				changed = true
			}
		}
	}

	return changed
}

// matchJobPidList get pid list related to the job
func (o *onlineManager) matchJobPidList(pidList []int, jobName string) []int {
	var matchedPidList []int
	var handlingPidList []int

	i := 1
	exp := o.jobSpecs[jobName].regexp
	for _, p := range pidList {
		handlingPidList = append(handlingPidList, p)
		if i%o.PidToCgroup.BatchNum == 0 {
			onlinePids := batchMatchPidList(handlingPidList, jobName, exp)
			if len(onlinePids) > 0 {
				matchedPidList = append(matchedPidList, onlinePids...)
			}
			// clear
			handlingPidList = []int{}
		}
		i++
	}
	if len(handlingPidList) != 0 {
		onlinePids := batchMatchPidList(handlingPidList, jobName, exp)
		if len(onlinePids) > 0 {
			matchedPidList = append(matchedPidList, onlinePids...)
		}
	}

	return matchedPidList
}

/*
updateJobSpec check if the cgroup for pids is fixed, such as managed by system service,
if not fixed, then move related pid list to target cgroup path.

11:net_cls:/
10:hugetlb:/
9:perf_event:/
8:blkio:/system.slice/system-ceph\x2dosd.slice
7:memory:/system.slice/system-ceph\x2dosd.slice
6:cpuset:/
5:devices:/system.slice/system-ceph\x2dosd.slice
4:cpu,cpuacct:/system.slice/system-ceph\x2dosd.slice
3:freezer:/
2:pids:/system.slice/system-ceph\x2dosd.slice
*/
func updateJobSpec(pidList []int, jobName string, spec *jobSpec) (added bool) {
	var needMovePidList []int
	nonFixedCgName := path.Join(types.CgroupNonK8sOnline, jobName)
	targetCgName := nonFixedCgName
	isSystemdService := false

	for _, pid := range pidList {
		func() {
			cgroups, err := cgroup.GetCgroupsByPid(pid)
			if err != nil {
				klog.Errorf("failed get cgroup for pid %d: %v", pid, err)
				return
			}
			cgName := cgroups[cgroup.MemorySubsystem]
			notFixed := cgroupNotFixed(cgName)
			if notFixed {
				cgName = nonFixedCgName
				needMovePidList = append(needMovePidList, pid)
			} else if cgroups[cgroup.CpuSetSubsystem] != cgName {
				// systemd service does not manage cpuset subsystem, need to move pid manually
				needMovePidList = append(needMovePidList, pid)
				targetCgName = cgName
				isSystemdService = true
			}

			newCg := &cgroupSpec{
				fixed:  !notFixed,
				pidNum: defaultPidNumber,
			}

			if _, ok := spec.cgroups[cgName]; !ok {
				klog.V(2).Infof("online job(%s) add new cgroup: %s(fixed=%v)",
					jobName, cgName, !notFixed)
				spec.cgroups[cgName] = newCg
				added = true
			}
		}()
	}

	if len(needMovePidList) != 0 {
		klog.V(3).Infof("moving pid list for job(%s) to target cgroup: %s", jobName, nonFixedCgName)
		if isSystemdService {
			moveOnlinePidList(targetCgName, needMovePidList, []string{cgroup.CpuSetSubsystem,
				cgroup.PerfEventSubsystem})
		} else {
			moveOnlinePidList(targetCgName, needMovePidList, []string{})
		}
	}

	return added
}

// moveOnlinePidList move online pids to some subsystem cgroups, now just as following:
// perf_event, memory, cpu,cpuacct, cpuset
func moveOnlinePidList(cgPath string, pids []int, subSystem []string) {
	err := cgroup.EnsureCgroupPath(cgPath)
	if err != nil {
		klog.Errorf("ensure online cgroup path(%s) err: %v", cgPath, err)
		return
	}
	err = cgroup.EnsureCpuSetCores(cgPath)
	if err != nil {
		klog.Errorf("failed ensure cpuset cores for %s: %v", cgPath, err)
		return
	}
	if len(subSystem) == 0 {
		subSystem = []string{cgroup.PerfEventSubsystem, cgroup.MemorySubsystem, cgroup.CPUSubsystem,
			cgroup.CpuSetSubsystem}
	}
	err = cgroup.MoveSpecificPids(subSystem, pids, cgPath)
	if err != nil {
		klog.Errorf("moving online pids to cgroup path(%s) err: %v", cgPath, err)
	}
}

// getMachinePidList get pid list from /proc
func getMachinePidList() []int {
	d, err := os.Open("/proc")
	if err != nil {
		klog.Fatalf("opem /proc err: %v", err)
	}
	defer d.Close()

	procs, err := d.Readdirnames(-1)
	if err != nil {
		klog.Fatalf("read /proc err: %v", err)
	}

	var pids []int
	for _, pStr := range procs {
		// just select the number
		if pStr[0] < '0' || pStr[0] > '9' {
			continue
		}
		if p, err := strconv.ParseUint(pStr, 10, 64); err == nil {
			pids = append(pids, int(p))
		}
	}

	return pids
}

/*
batchMatchPidList will grep multi process command line together witch regexp,
and choose the wanted process.

{"abc", "abcd", "cde"} => "abc+abcd+cde", indexs={{0,3},{4,8},{9,12}}
the regexp is "abc", then matcheds = {{0,2}, {4,8}}
*/
func batchMatchPidList(pidList []int, jobName string, exp *regexp.Regexp) []int {
	var onlinePids []int
	var cmdLines []string
	var indexs [][]int

	if len(pidList) == 0 {
		return onlinePids
	}

	// do not continue when looping pids, for we need to find pid from mapped index
	i := 0
	for _, p := range pidList {
		cmdlineFile := path.Join("/proc", fmt.Sprintf("%d", p), "cmdline")
		cmdLine, err := ioutil.ReadFile(cmdlineFile)
		if err != nil {
			klog.Errorf("read cmdline file(%s) err: %v", cmdlineFile, err)
			cmdLine = []byte{}
		} else {
			// command line, which read from proc, has replaced space character with null, so need to recover
			cmdLine = bytes.ReplaceAll(cmdLine, []byte{0}, []byte{32})
		}

		cmdLines = append(cmdLines, string(cmdLine))

		// get strings last index
		nextIndex := i + len(cmdLine)
		indexs = append(indexs, []int{i, nextIndex})
		i = nextIndex + 1
	}
	cmdLinesStr := strings.Join(cmdLines, "+")
	klog.V(4).Infof("command lines: %s", cmdLinesStr)

	matcheds := exp.FindAllIndex([]byte(cmdLinesStr), -1)
	if len(matcheds) == 0 {
		klog.V(3).Infof("regexp not matched for job: %s", jobName)
		return onlinePids
	}

	klog.V(4).Infof("command lines matcheds: %v", matcheds)
	klog.V(5).Infof("command lines indexs: %v", indexs)
	for p, index := range indexs {
		for _, match := range matcheds {
			if match[0] >= index[0] &&
				match[1] <= index[1] {
				onlinePids = append(onlinePids, pidList[p])
				break
			}
		}
	}
	klog.V(3).Infof("matched pid list for job(%s): %v", jobName, onlinePids)

	return onlinePids
}

func cgroupNotFixed(cgPath string) bool {
	switch cgPath {
	case "/", "/user.slice":
		return true
	default:
		if strings.Contains(cgPath, types.CgroupNonK8sOnline) {
			return true
		}
	}

	return false
}

func jobSpecsStr(jobSpecs map[string]*jobSpec) string {
	specStr := "online job specs:\n"
	for jobName, spec := range jobSpecs {
		for cg, cgSpec := range spec.cgroups {
			specStr += fmt.Sprintf("   job: %s, cgropu: %s, spec: %+v\n", jobName, cg, cgSpec)
		}
	}

	return specStr
}
