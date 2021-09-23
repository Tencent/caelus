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

package ring

// Ring is a ring buffer with fixed size.
type Ring interface {
	Add(v float64)
	Peek() (v float64)
	Values() []float64
	Ready() bool
	Mean() float64
}

type defaultRing struct {
	data     []float64
	capacity int
	tail     int
	ready    bool
}

// NewRing returns a ring buffer with fixed size.
func NewRing(n int) Ring {
	return &defaultRing{
		data:     make([]float64, n, n),
		capacity: n,
		ready:    false,
	}
}

// Add add values to ring
func (ring *defaultRing) Add(v float64) {
	i := ring.tail
	ring.data[i] = v
	ring.tail++
	if ring.tail == ring.capacity {
		ring.tail = 0
		ring.ready = true
	}
}

// Values return current ring buffer values
func (ring *defaultRing) Values() []float64 {
	return ring.data
}

// Peek returns latest value
func (ring *defaultRing) Peek() float64 {
	i := ring.tail - 1
	if i < 0 {
		i = ring.capacity - 1
	}
	return ring.data[i]
}

// Ready returns if the ring is ready to be used
func (ring *defaultRing) Ready() bool {
	return ring.ready
}

// Mean returns the mean value of current ring values
func (ring *defaultRing) Mean() float64 {
	var sum float64 = 0
	for _, v := range ring.data {
		sum += v
	}

	return sum / float64(ring.capacity)
}
