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
	"sort"

	"github.com/shirou/gopsutil/cpu"
	"k8s.io/klog"
)

var (
	threadSiblings map[int]int
)

// GenerateTheadSiblings will parse /proc/cpuinfo, and generate maps, showing which cores are belong to the same thead
// silbings. The map is nil when HT is disabled on the machine.
func GenerateTheadSiblings() error {
	cpuinfo, err := cpu.Info()
	if err != nil {
		return err
	}

	threadSiblings = make(map[int]int)

	coreIdToThead := make(map[string]int)
	for i := range cpuinfo {
		processorid := int(cpuinfo[i].CPU)
		nodeid := cpuinfo[i].PhysicalID
		coreid := cpuinfo[i].CoreID

		if tid, ok := coreIdToThead[nodeid+coreid]; !ok {
			coreIdToThead[nodeid+coreid] = processorid
		} else {
			threadSiblings[processorid] = tid
			threadSiblings[tid] = processorid
		}
	}
	klog.Infof("thread sibling generated: %v", threadSiblings)

	return nil
}

// ChooseNumaCores will choose number cores from total cores based on NUMA struct
func ChooseNumaCores(totalCores []int, chosenNum int) (chosen []int, left []int) {
	if len(totalCores) == 0 {
		return
	}
	if chosenNum == 0 {
		left = totalCores
		return
	}

	// sort the cores, and select from high to low
	sort.Ints(totalCores)

	ifChosen := make(map[int]bool)
	for _, c := range totalCores {
		ifChosen[c] = false
	}

	i := len(totalCores) - 1
	for {
		threadId := totalCores[i]
		if !ifChosen[threadId] {
			chosen = append(chosen, threadId)
			ifChosen[threadId] = true
			if len(chosen) == chosenNum {
				break
			}

			// also choose sibling thread
			if sibling, ok := threadSiblings[threadId]; ok {
				if selected, okk := ifChosen[sibling]; okk && !selected {
					chosen = append(chosen, sibling)
					ifChosen[sibling] = true
					if len(chosen) == chosenNum {
						break
					}
				}
			}
		}

		i--
		if i < 0 {
			break
		}
	}

	for _, c := range totalCores {
		if !ifChosen[c] {
			left = append(left, c)
		}
	}

	return chosen, left
}
