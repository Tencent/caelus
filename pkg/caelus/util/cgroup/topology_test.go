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
	"testing"
)

// testChoose group options for testing topology
type testChoose struct {
	describe       string
	threadSilbings map[int]int
	totalCores     []int
	chosenNum      int
	chosenCores    []int
	leftCores      []int
}

var testCases = []testChoose{
	{
		describe: "total cores is zero",
		threadSilbings: map[int]int{
			0: 3,
			1: 4,
			2: 5,
			3: 0,
			4: 1,
			5: 2,
		},
		totalCores:  []int{},
		chosenNum:   3,
		chosenCores: []int{},
		leftCores:   []int{},
	},
	{
		describe: "chosen number is zero",
		threadSilbings: map[int]int{
			0: 3,
			1: 4,
			2: 5,
			3: 0,
			4: 1,
			5: 2,
		},
		totalCores:  []int{1, 2, 3, 4, 5},
		chosenNum:   0,
		chosenCores: []int{},
		leftCores:   []int{1, 2, 3, 4, 5},
	},
	{
		describe:       "no thread silbing",
		threadSilbings: map[int]int{},
		totalCores:     []int{1, 2, 3, 4, 5},
		chosenNum:      3,
		chosenCores:    []int{3, 4, 5},
		leftCores:      []int{1, 2},
	},
	{
		describe: "more chosen numbers",
		threadSilbings: map[int]int{
			0: 3,
			1: 4,
			2: 5,
			3: 0,
			4: 1,
			5: 2,
		},
		totalCores:  []int{1, 2, 3, 4, 5},
		chosenNum:   7,
		chosenCores: []int{1, 2, 3, 4, 5},
		leftCores:   []int{},
	},
	{
		describe: "normal chosen numbers",
		threadSilbings: map[int]int{
			0: 3,
			1: 4,
			2: 5,
			3: 0,
			4: 1,
			5: 2,
		},
		totalCores:  []int{1, 2, 3, 4, 5},
		chosenNum:   3,
		chosenCores: []int{2, 4, 5},
		leftCores:   []int{1, 3},
	},
	{
		describe: "normal chosen numbers, just thread silbing",
		threadSilbings: map[int]int{
			0: 3,
			1: 4,
			2: 5,
			3: 0,
			4: 1,
			5: 2,
		},
		totalCores:  []int{1, 2, 3, 4, 5},
		chosenNum:   2,
		chosenCores: []int{2, 5},
		leftCores:   []int{1, 3, 4},
	},
}

// TestChooseNumaCores test choosing numa cores
func TestChooseNumaCores(t *testing.T) {
	for _, tc := range testCases {
		t.Logf("start testing case: %s", tc.describe)
		threadSiblings = tc.threadSilbings
		chosen, left := ChooseNumaCores(tc.totalCores, tc.chosenNum)
		sort.Ints(chosen)
		sort.Ints(left)

		if !checkSliceEqual(chosen, tc.chosenCores) {
			t.Fatalf("invalid chosen cores, should be: %v, but got: %v", tc.chosenCores, chosen)
		}
		if !checkSliceEqual(left, tc.leftCores) {
			t.Fatalf("invalid chosen cores, should be: %v, but got: %v", tc.leftCores, left)
		}
	}
}

func checkSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
