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

package action

import (
	"encoding/json"
	"gotest.tools/assert"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	testStepPercent          = 0.2
	testAdjustSelfDefineFunc = func(conflicting bool) *ActionResource {
		return &ActionResource{
			Name:             v1.ResourceCPU,
			Conflicting:      true,
			ConflictQuantity: map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-2000m")},
		}
	}
)

// TestActionAdjustOpLoop test adjust action operation "loop"
func TestActionAdjustOpLoop(t *testing.T) {
	args := adjustActionArgs{
		Op: opLoop,
		Resources: []adjustActionResourceArg{
			{
				Resource: v1.ResourceCPU,
				Step:     "1",
			},
			{
				Resource: v1.ResourceMemory,
				Step:     "512Mi",
			},
		},
	}
	argStr, _ := json.Marshal(args)
	adj := NewAdjustResourceAction(v1.ResourceCPU, argStr)
	ac, err := adj.DoAction(true, "cpu high")
	if err != nil {
		t.Fatalf("adjust action failed: %v", err)
	}
	aRes, exist := ac.AdjustResources[v1.ResourceCPU+"_"+v1.ResourceCPU]
	if !exist {
		t.Fatalf("expect adjust cpu but got %v", ac.AdjustResources)
	}
	assert.DeepEqual(t, aRes.ConflictQuantity,
		map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-1")})
	if len(ac.AdjustResources) != 1 {
		t.Fatalf("expect adjust one resource but got %v", ac.AdjustResources)
	}
	ac, err = adj.DoAction(true, "cpu high")
	if err != nil {
		t.Fatalf("adjust action failed: %v", err)
	}
	aRes, exist = ac.AdjustResources[v1.ResourceCPU+"_"+v1.ResourceMemory]
	if !exist {
		t.Fatalf("expect adjust memory but got %v", ac.AdjustResources)
	}
	assert.DeepEqual(t, aRes.ConflictQuantity,
		map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-512Mi")})
	if len(ac.AdjustResources) != 1 {
		t.Fatalf("expect adjust one resource but got %v", ac.AdjustResources)
	}
}

// TestActionAdjustOpAnd test adjust action operation "and"
func TestActionAdjustOpAnd(t *testing.T) {
	args := adjustActionArgs{
		Op: opAnd,
		Resources: []adjustActionResourceArg{
			{
				Resource: v1.ResourceCPU,
				Step:     "1",
			},
			{
				Resource: v1.ResourceMemory,
				Step:     "512Mi",
			},
		},
	}
	argStr, _ := json.Marshal(args)
	adj := NewAdjustResourceAction(v1.ResourceCPU, argStr)
	ac, err := adj.DoAction(true, "cpu high")
	if err != nil {
		t.Fatalf("adjust action failed: %v", err)
	}
	if len(ac.AdjustResources) != 2 {
		t.Fatalf("expect adjust one resource but got %v", ac.AdjustResources)
	}
	aRes, exist := ac.AdjustResources[v1.ResourceCPU+"_"+v1.ResourceCPU]
	if !exist {
		t.Fatalf("expect adjust cpu but got %v", ac.AdjustResources)
	}
	assert.DeepEqual(t, aRes.ConflictQuantity,
		map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-1")})
	aRes, exist = ac.AdjustResources[v1.ResourceCPU+"_"+v1.ResourceMemory]
	if !exist {
		t.Fatalf("expect adjust memory but got %v", ac.AdjustResources)
	}
	assert.DeepEqual(t, aRes.ConflictQuantity,
		map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-512Mi")})
}

var actionAdjustGenerateCases = []struct {
	describe    string
	adjRes      *adjustResource
	conflicting bool
	expect      *ActionResource
}{
	{
		describe: "FormulaStep with conflicting false",
		adjRes: &adjustResource{
			resource:     v1.ResourceCPU,
			step:         resource.MustParse("1000m"),
			negativeStep: resource.MustParse("-1000m"),
		},
		conflicting: false,
		expect: &ActionResource{
			Name:             v1.ResourceCPU,
			Conflicting:      false,
			ConflictQuantity: map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("1000m")},
		},
	},
	{
		describe: "FormulaStep with conflicting true",
		adjRes: &adjustResource{
			resource:     v1.ResourceCPU,
			step:         resource.MustParse("1000m"),
			negativeStep: resource.MustParse("-1000m"),
		},
		conflicting: true,
		expect: &ActionResource{
			Name:             v1.ResourceCPU,
			Conflicting:      true,
			ConflictQuantity: map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-1000m")},
		},
	},
	{
		describe: "FormulaPercent with conflicting false",
		adjRes: &adjustResource{
			resource:    v1.ResourceCPU,
			stepPercent: &testStepPercent,
			ratio:       0.64,
		},
		conflicting: false,
		expect: &ActionResource{
			Name:        v1.ResourceCPU,
			Conflicting: false,
			ConflictQuantity: map[ActionFormula]resource.Quantity{
				FormulaPercent: *resource.NewMilliQuantity(-200, resource.DecimalSI)},
		},
	},
	{
		describe: "FormulaPercent with conflicting true",
		adjRes: &adjustResource{
			resource:    v1.ResourceCPU,
			stepPercent: &testStepPercent,
			ratio:       1,
		},
		conflicting: true,
		expect: &ActionResource{
			Name:        v1.ResourceCPU,
			Conflicting: true,
			ConflictQuantity: map[ActionFormula]resource.Quantity{
				FormulaPercent: *resource.NewMilliQuantity(-200, resource.DecimalSI)},
		},
	},
	{
		describe: "Self handler",
		adjRes: &adjustResource{
			resource:              v1.ResourceCPU,
			actionResourceHandler: testAdjustSelfDefineFunc,
		},
		conflicting: false,
		expect: &ActionResource{
			Name:             v1.ResourceCPU,
			Conflicting:      true,
			ConflictQuantity: map[ActionFormula]resource.Quantity{FormulaStep: resource.MustParse("-2000m")},
		},
	},
}

// TestGenerateActionResource test generate handler for adjust action
func TestGenerateActionResource(t *testing.T) {
	for _, tCase := range actionAdjustGenerateCases {
		t.Logf("action adjust generate handler testing: %s", tCase.describe)

		acRes := generateActionResource(tCase.adjRes, tCase.conflicting)
		if !tCase.expect.Equal(acRes) {
			t.Fatalf("action adjust generate handler testing case(%s) failed, expect %+v, but got %+v",
				tCase.describe, tCase.expect, acRes)
		}
	}
}
