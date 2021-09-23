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

import (
	"math"
	"testing"
)

var epsilon = 0.00000001

func TestRing(t *testing.T) {
	ring := NewRing(5)
	data := []float64{1, 2, 3, 4}
	for _, d := range data {
		ring.Add(d)
	}

	cases := []struct {
		newData  float64
		expected []float64
	}{
		{
			newData:  5,
			expected: []float64{1, 2, 3, 4, 5},
		},
		{
			newData:  6,
			expected: []float64{6, 2, 3, 4, 5},
		},
		{
			newData:  7,
			expected: []float64{6, 7, 3, 4, 5},
		},
	}

	for _, test := range cases {
		ring.Add(test.newData)
		if !isEqualFloat64(ring.Peek(), test.newData) {
			t.Errorf("Failed to peek, expected %+v, got %+v", test.newData, ring.Peek())
		}
		if !isEqual(ring.Values(), test.expected) {
			t.Errorf("Failed, expected %+v, got %+v", test.expected, ring.Values())
		}
	}
}

func isEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !isEqualFloat64(a[i], b[i]) {
			return false
		}
	}
	return true
}

func isEqualFloat64(a, b float64) bool {
	// The following algorithm is not right, but it is OK for test now.
	// For more information, refer https://floating-point-gui.de/errors/comparison/#look-out-for-edge-cases.
	return math.Abs(a-b) < epsilon
}
