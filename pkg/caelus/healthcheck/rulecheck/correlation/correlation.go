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

package correlation

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/tencent/caelus/pkg/caelus/statestore"
	cgroupstore "github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// Analysis analysis the source instances that affect victim instances
func Analysis(metric string, victimStats map[cgroupstore.CgroupRef]*cgroupstore.CgroupStats,
	stat statestore.StateStore, podInformer cache.SharedIndexInformer) *cgroupstore.CgroupRef {
	switch metric {
	case "memory":
		// TODO: filter out victims whose memory usage exceed request
		suspect := findSuspect(victimStats, memLoadFunc(podInformer), memFilter, stat, podInformer)
		return suspect
	case "diskio":
		// TODO, need to implement
		return nil
	default:
		return nil
	}
}

type loadFunc func(s *cgroupstore.CgroupStats) float64

// memLoadFunc construct pod memory request and usage function
func memLoadFunc(podInformer cache.SharedIndexInformer) loadFunc {
	return func(s *cgroupstore.CgroupStats) float64 {
		req := getPodMemoryRequest(s.Ref, podInformer)
		if req == 0 {
			return 0
		}
		usage := s.MemoryTotalUsage / (2 ^ 30)
		return (usage - req) / req
	}
}

type filterFunc func(maxPri int, minLoad float64, currentPri int, currentLoad float64,
	cgroupStats *cgroupstore.CgroupStats) bool

// memFilter filter pods with less resource usage
func memFilter(maxPri int, minLoad float64, currentPri int, currentLoad float64,
	cgroupStats *cgroupstore.CgroupStats) bool {
	if currentLoad < 0 {
		return false
	}
	if cgroupStats.MemoryTotalUsage/(2^30) < 0.5 {
		return false
	}
	if currentPri < maxPri || (maxPri == currentPri && minLoad < currentLoad) {
		return true
	}
	return false
}

// findSuspect find suspicious pods based on resource usage
func findSuspect(victimStats map[cgroupstore.CgroupRef]*cgroupstore.CgroupStats, loadFunc loadFunc,
	filter filterFunc, stat statestore.StateStore, podInformer cache.SharedIndexInformer) *cgroupstore.CgroupRef {
	statMap, err := stat.ListCgroupResourceRecentState(false, sets.NewString())
	if err != nil {
		return nil
	}
	var suspects []suspect

	// get max priority and min load
	maxPri := 0
	minLoad := 0.0
	for ref, cstat := range victimStats {
		pri := getPodPriority(ref, podInformer)
		if pri == maxPri {
			load := loadFunc(cstat)
			if load < minLoad {
				minLoad = load
			}
		}
		if pri > maxPri {
			minLoad = loadFunc(cstat)
			maxPri = pri
		}
	}
	for _, cstat := range statMap {
		if cstat.Ref.PodNamespace == "" && cstat.Ref.AppClass == appclass.AppClassOnline {
			continue
		}
		pri := getPodPriority(*cstat.Ref, podInformer)
		load := loadFunc(cstat)
		// filter out pods whose priority is higher than current priority but load is lower
		if filter(maxPri, minLoad, pri, load, cstat) {
			suspects = append(suspects, suspect{
				ref:      cstat.Ref,
				priority: pri,
				val:      load,
				runtime:  getPodRunningTime(*cstat.Ref, podInformer),
			})
		}
	}
	if len(suspects) == 0 {
		return nil
	}
	// sort by priority and value
	sort.Slice(suspects, func(i, j int) bool {
		if suspects[i].priority == suspects[j].priority {
			return suspects[i].val > suspects[j].val
		}
		return suspects[i].priority < suspects[j].priority
	})
	// try to choose pods just starting recently
	var recentFilter = func(items []suspect) []suspect {
		var cand []suspect
		for _, item := range items {
			if item.runtime < 15*time.Minute {
				cand = append(cand, item)
			}
		}
		if len(cand) > 0 {
			return cand
		}
		return items
	}
	suspects = recentFilter(suspects)
	return suspects[0].ref
}

// getPodPriority get pod priority
func getPodPriority(ref cgroupstore.CgroupRef, podInformer cache.SharedIndexInformer) int {
	if ref.PodNamespace == "" {
		if ref.AppClass == appclass.AppClassOnline {
			// non k8s online
			return math.MaxInt32
		}
		return math.MinInt32
	}
	item, exist, err := podInformer.GetStore().GetByKey(fmt.Sprintf("%s/%s", ref.PodNamespace, ref.PodName))
	if err != nil || !exist {
		return 0
	}

	pod := item.(*v1.Pod)
	if pod.Spec.Priority != nil {
		return int(*pod.Spec.Priority)
	}
	return 0
}

// getPodRunningTime get how long the pod has running
func getPodRunningTime(ref cgroupstore.CgroupRef, podInformer cache.SharedIndexInformer) time.Duration {
	if ref.PodNamespace == "" {
		return 24 * time.Hour
	}
	item, exist, err := podInformer.GetStore().GetByKey(fmt.Sprintf("%s/%s", ref.PodNamespace, ref.PodName))
	if err != nil || !exist {
		return 0
	}
	pod := item.(*v1.Pod)
	if pod.Status.StartTime == nil {
		return 0
	}
	return time.Since(pod.Status.StartTime.Time)
}

// getPodMemoryRequest get pod memory request value
func getPodMemoryRequest(ref *cgroupstore.CgroupRef, podInformer cache.SharedIndexInformer) float64 {
	item, exist, err := podInformer.GetStore().GetByKey(fmt.Sprintf("%s/%s", ref.PodNamespace, ref.PodName))
	if err != nil || !exist {
		return 0
	}
	pod := item.(*v1.Pod)
	for _, c := range pod.Spec.Containers {
		if c.Name == ref.ContainerName {
			mem := c.Resources.Requests.Memory()
			if mem == nil {
				return 0
			}
			memV := mem.ScaledValue(resource.Mega)
			return float64(memV) / 1024
		}
	}
	return 0
}
