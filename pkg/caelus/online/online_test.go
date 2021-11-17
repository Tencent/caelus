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
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/statestore/mock"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	"github.com/opencontainers/runc/libcontainer/cgroups"
)

type onlineTestData struct {
	describe       string
	fixed          bool
	config         types.OnlineConfig
	expectExtraCgs []string
	expectSpec     map[string]*jobSpec
}

var (
	cgroupTestPath = "/testing"
)

// TestGenerateOnlineCgroups tests generate online cgroups cases
func TestGenerateOnlineCgroups(t *testing.T) {
	onlineTestCases := []onlineTestData{
		{
			describe: "non fixed cgroup",
			fixed:    false,
			config: types.OnlineConfig{
				Jobs: []types.OnlineJobConfig{
					{
						Name:    "nonFixedCgroup",
						Command: "go test",
					},
				},
				PidToCgroup: types.PidToCgroup{
					BatchNum: 10,
				},
			},
			expectExtraCgs: []string{
				"/onlinejobs",
			},
			expectSpec: map[string]*jobSpec{
				"nonFixedCgroup": {
					cgroups: map[string]*cgroupSpec{
						"/onlinejobs/nonFixedCgroup": {
							fixed:  false,
							pidNum: -1,
						},
					},
				},
			},
		},
		{
			describe: "fixed cgroup",
			fixed:    true,
			config: types.OnlineConfig{
				Jobs: []types.OnlineJobConfig{
					{
						Name:    "fixedCgroup",
						Command: "go test",
					},
				},
				PidToCgroup: types.PidToCgroup{
					BatchNum: 10,
				},
			},
			expectExtraCgs: []string{
				cgroupTestPath,
			},
			expectSpec: map[string]*jobSpec{
				"fixedCgroup": {
					cgroups: map[string]*cgroupSpec{
						cgroupTestPath: {
							fixed:  true,
							pidNum: -1,
						},
					},
				},
			},
		},
		{
			describe: "signal pid number match",
			fixed:    true,
			config: types.OnlineConfig{
				Jobs: []types.OnlineJobConfig{
					{
						Name:    "fixedCgroup",
						Command: "go test",
					},
				},
				PidToCgroup: types.PidToCgroup{
					BatchNum: 1,
				},
			},
			expectExtraCgs: []string{
				cgroupTestPath,
			},
			expectSpec: map[string]*jobSpec{
				"fixedCgroup": {
					cgroups: map[string]*cgroupSpec{
						cgroupTestPath: {
							fixed:  true,
							pidNum: -1,
						},
					},
				},
			},
		},
	}

	// the parent process is "go test"
	currentPid := os.Getppid()
	err := cgroup.EnsureCgroupPath(cgroupTestPath)
	if err != nil {
		t.Fatalf("online test failed for mkdir cgroup err: %v", err)
	}
	err = cgroup.EnsureCpuSetCores(cgroupTestPath)
	if err != nil {
		t.Fatalf("online test failed for set cpuset err: %v", err)
	}
	defer func() {
		cgs, _ := cgroups.GetAllSubsystems()
		rootPath := cgroup.GetRoot()
		for _, cg := range cgs {
			os.RemoveAll(path.Join(rootPath, cg, cgroupTestPath))
		}
	}()
	// no need to check the error, for the above EnsureCgroupPath has passed
	subsystems, _ := cgroups.GetAllSubsystems()

	for _, oCase := range onlineTestCases {
		if oCase.fixed {
			err = cgroup.MoveSpecificPids(subsystems, []int{currentPid}, cgroupTestPath)
		} else {
			err = cgroup.MoveSpecificPids(subsystems, []int{currentPid}, "/")
		}
		if err != nil {
			t.Logf("online test case %s move pids err: %v, just skip the case", oCase.describe, err)
			continue
		}

		cgSt := &mock.MockCgroupStore{}
		st := mock.NewMockStatStore(nil, cgSt)
		onManager := mockOnlineManager(oCase.config, st)
		onManager.generateOnlineCgroups()

		if !checkSliceEqual(cgSt.ExtraCgs, oCase.expectExtraCgs) {
			t.Fatalf("online test case %s failed, extra cgroups expect %v, got %v",
				oCase.describe, oCase.expectExtraCgs, cgSt.ExtraCgs)
		}
		if !checkMapEqual(onManager.jobSpecs, oCase.expectSpec) {
			t.Fatalf("online test case %s failed, job specs expect %s, got %s",
				oCase.describe, jobSpecsStr(oCase.expectSpec), jobSpecsStr(onManager.jobSpecs))
		}
	}
}

func mockOnlineManager(config types.OnlineConfig, stStore statestore.StateStore) *onlineManager {
	online := &onlineManager{
		OnlineConfig: config,
		stStore:      stStore,
		jobSpecs:     make(map[string]*jobSpec),
	}

	// generate cgroup spec
	for _, job := range config.Jobs {
		exp, _ := regexp.Compile(job.Command)
		online.jobSpecs[job.Name] = &jobSpec{
			regexp:  exp,
			cgroups: make(map[string]*cgroupSpec),
		}
	}

	return online
}

func checkSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]bool)
	for _, v := range a {
		aMap[v] = true
	}
	for _, v := range b {
		if _, ok := aMap[v]; !ok {
			return false
		}
	}

	return true
}

func checkMapEqual(a, b map[string]*jobSpec) bool {
	if len(a) != len(b) {
		return false
	}

	for k, va := range a {
		if vb, ok := b[k]; !ok {
			return false
		} else {
			if len(va.cgroups) != len(vb.cgroups) {
				return false
			}
			for ka, vva := range va.cgroups {
				if vvb, ok := vb.cgroups[ka]; !ok {
					return false
				} else {
					if vva.fixed != vvb.fixed {
						return false
					}
					if vva.pidNum != vvb.pidNum {
						return false
					}
				}
			}
		}
	}

	return true
}
