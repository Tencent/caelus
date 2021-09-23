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

package cpi

import (
	"sort"
	"time"
)

// timeSeries stores data and sort them by timestamp
type timeSeries struct {
	timeline []int64
	data     map[int64]float64
}

func newTimeSeries() *timeSeries {
	return &timeSeries{
		data: make(map[int64]float64),
	}
}

func (t *timeSeries) add(value float64, timestamp time.Time) {
	t.data[timestamp.UnixNano()] = value
	t.timeline = append(t.timeline, timestamp.UnixNano())
	sort.Sort(int64Asc(t.timeline))
}

func (t *timeSeries) getByUnixNano(nano int64) (float64, bool) {
	v, ok := t.data[nano]
	return v, ok
}

// rangeSearch output data series between the start and end timestamp
func (t *timeSeries) rangeSearch(start, end time.Time) *timeSeries {
	if end.Before(start) {
		return nil
	}

	startIndex := sort.Search(len(t.timeline), func(i int) bool {
		return t.timeline[i] >= start.UnixNano()
	})
	if startIndex == len(t.timeline) {
		return nil
	}
	endIndex := sort.Search(len(t.timeline), func(i int) bool {
		return t.timeline[i] >= end.UnixNano()
	})
	if endIndex < len(t.timeline) && t.timeline[endIndex] == end.UnixNano() {
		endIndex++
	}
	timeRange := t.timeline[startIndex:endIndex]

	ret := newTimeSeries()
	for _, ts := range timeRange {
		v, ok := t.data[ts]
		if !ok {
			continue
		}
		ret.add(v, time.Unix(0, ts))
	}
	return ret
}

// normalize normalize the data value to 1
func (t timeSeries) normalize() *timeSeries {
	var sum float64
	for _, v := range t.data {
		sum += v
	}
	ret := newTimeSeries()
	for tl, v := range t.data {
		ret.add(v/sum, time.Unix(0, tl))
	}
	return ret
}

// expire delete expired data
func (t *timeSeries) expire(before time.Time) {
	i := sort.Search(len(t.timeline), func(i int) bool {
		return t.timeline[i] >= before.UnixNano()
	})
	expired := t.timeline[:i]
	if i == len(t.timeline) {
		t.timeline = []int64{}
	} else {
		t.timeline = t.timeline[i:]
	}
	for _, e := range expired {
		delete(t.data, e)
	}
}

type int64Asc []int64

// Len returns slice length
func (i64 int64Asc) Len() int {
	return len(i64)
}

// Less compares slice elements
func (i64 int64Asc) Less(i, j int) bool {
	return i64[i] < i64[j]
}

// Swap swaps slice elements
func (i64 int64Asc) Swap(i, j int) {
	i64[i], i64[j] = i64[j], i64[i]
}
