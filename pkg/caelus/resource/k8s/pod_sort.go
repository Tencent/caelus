/*
 * Copyright The Kubernetes Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 *
 * For how to sort pod, we learn something from
 * https://github.com/kubernetes/kubernetes/pkg/kubelet/eviction/helpers.go
 */

package k8s

import (
	"sort"

	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/apis/scheduling"
)

// Cmp compares p1 and p2 and returns:
//
//	-1 if p1 <  p2
//	 0 if p1 == p2
//	+1 if p1 >  p2
type cmpFunc func(p1, p2 *v1.Pod) int

// multiSorter implements the Sort interface, sorting changes within.
type multiSorter struct {
	pods []*v1.Pod
	cmp  []cmpFunc
}

// Sort sorts the argument slice according to the less functions passed to OrderedBy.
func (ms *multiSorter) Sort(pods []*v1.Pod) {
	ms.pods = pods
	sort.Sort(ms)
}

// OrderedBy returns a Sorter that sorts using the cmp functions, in order.
// Call its Sort method to sort the data.
func OrderedBy(cmp ...cmpFunc) *multiSorter {
	return &multiSorter{
		cmp: cmp,
	}
}

// Len is part of sort.Interface
func (ms *multiSorter) Len() int {
	return len(ms.pods)
}

// Swap is part of sort.Interface
func (ms *multiSorter) Swap(i, j int) {
	ms.pods[i], ms.pods[j] = ms.pods[j], ms.pods[i]
}

// Less is part of sort.Interface
func (ms *multiSorter) Less(i, j int) bool {
	p1, p2 := ms.pods[i], ms.pods[j]
	var k int
	for k = 0; k < len(ms.cmp)-1; k++ {
		cmpResult := ms.cmp[k](p1, p2)
		// p1 is less than p2
		if cmpResult < 0 {
			return true
		}
		// p1 is greater than p2
		if cmpResult > 0 {
			return false
		}
		// we don't know yet
	}
	// the last cmp func is the final decider
	return ms.cmp[k](p1, p2) < 0
}

// SortByPriority compare pods by Priority
func SortByPriority(p1, p2 *v1.Pod) int {
	priority1 := getPodPriority(p1)
	priority2 := getPodPriority(p2)
	if priority1 == priority2 {
		return 0
	}
	if priority1 > priority2 {
		return 1
	}
	return -1
}

// getPodPriority return priority of the given pod
func getPodPriority(pod *v1.Pod) int32 {
	if pod.Spec.Priority != nil {
		return *pod.Spec.Priority
	}
	// When priority of a running pod is nil, it means it was created at a time
	// that there was no global default priority class and the priority class
	// name of the pod was empty. So, we resolve to the static default priority.
	return scheduling.DefaultPriorityWhenNoDefaultClassExists
}

// SortByResource compare pods by resource usage
// Now just compare with usage quantity, not compare with request
func SortByResource(usageMap map[string]int64) cmpFunc {
	return func(p1, p2 *v1.Pod) int {
		p1Name := GetPodKey(p1)
		p2Name := GetPodKey(p2)

		r1, ok1 := usageMap[p1Name]
		r2, ok2 := usageMap[p2Name]

		if !ok1 && !ok2 {
			return 0
		}
		if !ok1 {
			return -1
		}
		if !ok2 {
			return 1
		}

		return int(r2 - r1)
	}
}

// SortByStartTime compare pods by the running duration
func SortByStartTime(p1, p2 *v1.Pod) int {
	t1 := p1.Status.StartTime
	t2 := p2.Status.StartTime

	if t1 == nil && t2 == nil {
		return 0
	}
	if t1 == nil {
		// maybe t1 is just starting
		return 1
	}
	if t2 == nil {
		return -1
	}

	if t1.Before(t2) {
		return 1
	} else if t1.Equal(t2) {
		return 0
	}
	return -1
}
