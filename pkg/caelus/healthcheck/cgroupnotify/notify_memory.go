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

package notify

import (
	"fmt"
	"path"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
	"github.com/tencent/caelus/pkg/util/times"

	"k8s.io/klog"
)

var (
	memoryNotifierName      = "memory_cgroup_notifier"
	memoryUsageAttribute    = "memory.usage_in_bytes"
	memoryPressureAttribute = "memory.pressure_level"

	minDroppedCache = 5120 * int(types.MemUnit)
)

var _ notifier = &memoryNotifier{}

// memoryNotifier describe memory cgroup notify data
type memoryNotifier struct {
	cgroupNotifiers []cgroupNotifier
}

// newMemoryNotifier creates memory notify instance
func newMemoryNotifier(cfg *types.MemoryNotifyConfig, handle func()) notifier {
	cgroupRoot := cgroup.GetRoot()
	var cgroupNotifiers []cgroupNotifier

	cgroupNotifiers = append(cgroupNotifiers, generateMemoryPressureNotifier(cgroupRoot, cfg.Pressures, handle)...)
	cgroupNotifiers = append(cgroupNotifiers, generateMemoryUsageNotifier(cgroupRoot, cfg.Usages, handle)...)

	return &memoryNotifier{
		cgroupNotifiers: cgroupNotifiers,
	}
}

func (m *memoryNotifier) name() string {
	return memoryNotifierName
}

func (m *memoryNotifier) start(stopCh <-chan struct{}) error {
	if len(m.cgroupNotifiers) == 0 {
		klog.V(2).Infof("no %s found, no need to listen", memoryNotifierName)
		return nil
	}

	for _, cgNf := range m.cgroupNotifiers {
		// must copy the pointer, or the goroutine will use wrong cgroup notifiy
		nf := cgNf
		go nf.start()
		// start listening event
		go nf.eventHandleFunc()
	}
	go func() {
		select {
		case <-stopCh:
			_ = m.stop()
		}
	}()
	return nil
}

func (m *memoryNotifier) stop() error {
	for _, cgNf := range m.cgroupNotifiers {
		cgNf.stop()
	}
	return nil
}

// generateMemoryPressureNotifier init memory.pressure notify.
func generateMemoryPressureNotifier(cgRootPath string,
	pressureCfgs []types.MemoryPressureNotifyConfig, outerHandleFunc func()) []cgroupNotifier {
	var pressureNotifiers []cgroupNotifier
	for _, pressureCgs := range pressureCfgs {
		for _, cg := range pressureCgs.Cgroups {
			cgPath := path.Join(cgRootPath, cgroup.MemorySubsystem, cg)

			cgNotifier, err := newCgroupNotify(cgPath, memoryPressureAttribute, pressureCgs.PressureLevel)
			if err != nil {
				klog.Errorf("create cgroup memory notifier failed for path(%s) with attribute(%s): %v",
					cg, memoryPressureAttribute, err)
				continue
			}
			klog.V(2).Infof("memory cgroup(%s) notifier with attribute(%s) create successfully",
				cg, memoryPressureAttribute)

			data := commonCgroupData{
				cgroup:          cg,
				attribute:       memoryPressureAttribute,
				duration:        pressureCgs.Duration,
				notify:          cgNotifier,
				events:          make(chan struct{}),
				outerHandleFunc: outerHandleFunc,
			}
			pressureNotifiers = append(pressureNotifiers, newCgroupPressureNotifier(data, pressureCgs.Count))
		}
	}
	return pressureNotifiers
}

// generateMemoryUsageNotifier init memory.usage_in_bytes notify,
// receiving events when the threshold is crossed in either direction.
func generateMemoryUsageNotifier(cgRootPath string,
	usageCfgs []types.MemoryUsageNotifyConfig, outerHandleFunc func()) []cgroupNotifier {
	var usageNotifiers []cgroupNotifier
	for _, usageCgs := range usageCfgs {
		for _, cg := range usageCgs.Cgroups {
			limitSize, limited, err := cgroup.GetMemoryLimit(cg)
			if err != nil {
				klog.Errorf("get memory cgroup from %s limit value err: %v", cg, err)
				continue
			}
			if !limited {
				klog.Warningf("memory cgroup(%s) has no limit size, ignore usage notifier", cg)
				continue
			}

			cgPath := path.Join(cgRootPath, cgroup.MemorySubsystem, cg)
			threshold := limitSize - usageCgs.MarginMb*int(types.MemUnit)
			if threshold <= 0 {
				klog.Fatalf("too large margin size(%d), limit size is %d",
					usageCgs.MarginMb*int(types.MemUnit), limitSize)
			}

			cgNotifier, err := newCgroupNotify(cgPath, memoryUsageAttribute, fmt.Sprintf("%d", threshold))
			if err != nil {
				klog.Errorf("memory cgroup(%s) notifier create failed with attribute(%s:%d): %v",
					cg, memoryUsageAttribute, threshold, err)
				continue
			}
			klog.V(2).Infof("memory cgroup(%s) notifier with attribute(%s:%d) create successfully",
				cg, memoryUsageAttribute, threshold)

			data := commonCgroupData{
				cgroup:          cg,
				attribute:       memoryUsageAttribute,
				duration:        usageCgs.Duration,
				notify:          cgNotifier,
				events:          make(chan struct{}),
				outerHandleFunc: outerHandleFunc,
			}
			usageNotifiers = append(usageNotifiers, newCgroupUsageNotifier(data, threshold))
		}
	}
	return usageNotifiers
}

// commonCgroupData describes cgoup notify variables
type commonCgroupData struct {
	cgroup          string
	attribute       string
	duration        times.Duration
	notify          *linuxCgroupNotify
	outerHandleFunc func()
	events          chan struct{}
}

// cgroupNotifier is the cgroup notify interface
type cgroupNotifier interface {
	start()
	stop()
	eventHandleFunc()
}

// cgroupPressureNotifier describes memory.pressure_level cgroup notify interface
type cgroupPressureNotifier struct {
	commonCgroupData
	count int
}

// newCgroupPressureNotifier news memory.pressure_level cgroup notify instance
func newCgroupPressureNotifier(data commonCgroupData, count int) cgroupNotifier {
	return &cgroupPressureNotifier{
		commonCgroupData: data,
		count:            count,
	}
}

func (p *cgroupPressureNotifier) start() {
	p.notify.start(p.events)
	klog.V(2).Infof("%s starting to listen for cgroup %s with attribute %s",
		memoryNotifierName, p.cgroup, p.attribute)
}

func (p *cgroupPressureNotifier) stop() {
	p.notify.stop()
	klog.V(2).Infof("%s starting to stop cgroup %s with attribute %s",
		memoryNotifierName, p.cgroup, p.attribute)
}

// eventHandleFunc will receiving continuous events when memory is under reclaiming.
func (p *cgroupPressureNotifier) eventHandleFunc() {
	validDuration := p.duration.TimeDuration()
	// invalidDuration used to check if the duration is too long, just ignore
	invalidDuration := p.duration.TimeDuration() + p.duration.TimeDuration()
	// record event number
	var count int
	// record the event start timestamp
	var lastTick *time.Time
	for range p.events {
		klog.V(2).Infof("receiving memory cgroup(%s) with attribute %s event signal",
			p.cgroup, p.attribute)

		// if the previous event is too old, dropping the previous events
		if lastTick == nil || lastTick.Add(invalidDuration).Before(time.Now()) {
			current := time.Now()
			lastTick = &current
			count = 0
		}
		count++

		if lastTick.Add(validDuration).After(time.Now()) {
			continue
		}
		if count < p.count {
			klog.Warningf("memory cgroup(%s) pressure is still happening for %v seconds,"+
				"while the event number(%d) is less than %d, nothing to do",
				p.cgroup, p.duration.Seconds(), count, p.count)
			lastTick = nil
			continue
		}

		klog.V(2).Infof("memory cgroup(%s) pressure has kept for %v seconds with events number %d,"+
			"calling handle function", p.cgroup, p.duration.Seconds(), count)
		p.outerHandleFunc()
		lastTick = nil
	}
}

// cgroupUsageNotifier describes memory.usage_in_bytes cgroup notify interface
type cgroupUsageNotifier struct {
	commonCgroupData
	threshold int
}

// newCgroupUsageNotifier news memory.usage_in_bytes cgroup notify instance
func newCgroupUsageNotifier(data commonCgroupData, threshold int) cgroupNotifier {
	return &cgroupUsageNotifier{
		commonCgroupData: data,
		threshold:        threshold,
	}
}

func (u *cgroupUsageNotifier) start() {
	u.notify.start(u.events)
	klog.V(2).Infof("%s starting to listen for cgroup %s with attribute %s",
		memoryNotifierName, u.cgroup, u.attribute)
}

func (u *cgroupUsageNotifier) stop() {
	u.notify.stop()
	klog.V(2).Infof("%s starting to stop cgroup %s with attribute %s",
		memoryNotifierName, u.cgroup, u.attribute)
}

// eventHandleFunc checks if the usage is still above threshold after receiving event,
// try drop cache firstly, and call handle function if releasing little cache.
func (u *cgroupUsageNotifier) eventHandleFunc() {
	var handleTicker *time.Timer

	// wrapper function, try drop cache, then call handle function
	handleFuncWrapper := func() {
		currentUsage, err := cgroup.GetMemoryUsage(u.cgroup)
		if err != nil {
			klog.Errorf("get memory cgroup %s usage err: %v", u.cgroup, err)
			return
		}
		// the usage may be downing, crossing the threshold
		if currentUsage < u.threshold {
			klog.V(2).Infof("memory cgroup %s usage has down, no need to handle", u.cgroup)
			return
		}

		// usage has exceed threshold, do some thing! try drop cache firstly
		klog.V(2).Infof("memory cgroup(%s) usage is high for %v seconds, dropping cache",
			u.cgroup, u.duration.Seconds())
		start := time.Now()
		supported, err := cgroup.MemoryForceEmpty(u.cgroup)
		if err != nil {
			klog.Errorf("cgroup(%s) drop cache err: %v", u.cgroup, err)
		} else if supported {
			klog.V(2).Infof("cgroup(%s) dropping cache costing time: %v",
				u.cgroup, time.Now().Sub(start))
			newestUsage, err := cgroup.GetMemoryUsage(u.cgroup)
			if err != nil {
				klog.Errorf("get memory cgroup %s usage err: %v", u.cgroup, err)
			} else {
				if newestUsage >= u.threshold {
					klog.Warningf("memory cgroup(%s) usage is still above threshold after dropping cache,"+
						"calling handle function", u.cgroup)
				} else {
					if currentUsage-newestUsage > minDroppedCache ||
						newestUsage <= currentUsage/2 {
						klog.V(2).Infof("cgroup(%s) has dropped most of cache: %d", u.cgroup,
							currentUsage-newestUsage)
						return
					}
				}
			}
		} else {
			klog.V(2).Infof("drop cgroup cache not supported, calling handle function")
		}

		// call handle function if just dropping little pages
		klog.V(2).Infof("memory cgroup(%s) usage is still high after dropping cache,"+
			"calling handle function", u.cgroup)
		u.outerHandleFunc()
	}

	for range u.events {
		klog.V(2).Infof("receiving memory cgroup(%s) with attribute %s event signal",
			u.cgroup, u.attribute)

		if handleTicker == nil {
			handleTicker = time.AfterFunc(u.duration.TimeDuration(), handleFuncWrapper)
		} else {
			handleTicker.Reset(u.duration.TimeDuration())
		}
	}
}
