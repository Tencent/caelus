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
	"regexp"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/statestore/mock"
	"github.com/tencent/caelus/pkg/caelus/types"
)

type onlineTestData struct {
	describe       string
	config         types.OnlineConfig
	expectExtraCgs []string
	expectSpec     map[string]*jobSpec
}

// TestGenerateOnlineCgroups tests generate online cgroups cases
func TestGenerateOnlineCgroups(t *testing.T) {
	onlineTestCases := []onlineTestData{
		{
			describe: "system service",
			config: types.OnlineConfig{
				Jobs: []types.OnlineJobConfig{
					{
						Name:    "journal",
						Command: "systemd-journald",
					},
				},
				PidToCgroup: types.PidToCgroup{
					BatchNum: 10,
				},
			},
			expectExtraCgs: []string{
				"/system.slice/systemd-journald.service",
			},
			expectSpec: map[string]*jobSpec{
				"journal": {
					cgroups: map[string]*cgroupSpec{
						"/system.slice/systemd-journald.service": {
							fixed:  true,
							pidNum: -1,
						},
					},
				},
			},
		},
		{
			describe: "daemon process service",
			config: types.OnlineConfig{
				Jobs: []types.OnlineJobConfig{
					{
						Name:    "testing",
						Command: "go test -v -count=1",
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
				"testing": {
					cgroups: map[string]*cgroupSpec{
						"/onlinejobs/testing": {
							fixed:  false,
							pidNum: -1,
						},
					},
				},
			},
		},
		{
			describe: "signal pid number match",
			config: types.OnlineConfig{
				Jobs: []types.OnlineJobConfig{
					{
						Name:    "journal",
						Command: "systemd-journald",
					},
				},
				PidToCgroup: types.PidToCgroup{
					BatchNum: 1,
				},
			},
			expectExtraCgs: []string{
				"/system.slice/systemd-journald.service",
			},
			expectSpec: map[string]*jobSpec{
				"journal": {
					cgroups: map[string]*cgroupSpec{
						"/system.slice/systemd-journald.service": {
							fixed:  true,
							pidNum: -1,
						},
					},
				},
			},
		},
	}

	for _, oCase := range onlineTestCases {
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
