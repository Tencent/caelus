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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

const (
	opAnd  = "and"
	opLoop = "loop"
)

type AdjustSelfDefineFunc func(conflicting bool) *ActionResource

type adjustActionArgs struct {
	Op        string                    `json:"op"`
	Resources []adjustActionResourceArg `json:"resources"`
}

type adjustActionResourceArg struct {
	Resource    v1.ResourceName `json:"resource"`
	Step        string          `json:"step"`
	StepPercent *float64        `json:"step_percent"`
}

// AdjustResourceAction get Action instance to adjust offline resources
type AdjustResourceAction struct {
	resource        v1.ResourceName
	adjIndex        int
	op              string
	adjResKind      int
	adjustResources []*adjustResource
}

type adjustResource struct {
	resource v1.ResourceName
	step     resource.Quantity
	// negative for conflicting resource
	negativeStep resource.Quantity
	stepPercent  *float64
	// ratio show the non-conflicting resource
	ratio                 float64
	actionResourceHandler AdjustSelfDefineFunc
}

// NewAdjustResourceAction return AdjustResourceAction
func NewAdjustResourceAction(resourceName v1.ResourceName, args []byte) Action {
	adjAction := &AdjustResourceAction{
		resource: resourceName,
	}

	argsData := &adjustActionArgs{}
	err := json.Unmarshal(args, argsData)
	if err != nil {
		klog.Fatalf("invalid args(%s) for adjust resource action: %v", string(args), err)
	}

	adjAction.op = argsData.Op
	if len(adjAction.op) == 0 {
		adjAction.op = opAnd
	}

	var adjustResources []*adjustResource
	for _, adjRes := range argsData.Resources {
		if len(adjRes.Step) != 0 && adjRes.StepPercent != nil {
			klog.Fatalf("step and step_percent must be set only one for adjust resource action: %s", string(args))
		}
		if len(adjRes.Step) == 0 {
			adjRes.Step = "0"
			if adjRes.StepPercent == nil {
				klog.Warningf("both step and step_percent is nil, set default zero value for %s", resourceName)
			}
		}
		adjResName := adjRes.Resource
		if len(adjResName) == 0 {
			adjResName = resourceName
		}

		adjustResources = append(adjustResources, &adjustResource{
			resource:     adjResName,
			step:         resource.MustParse(adjRes.Step),
			negativeStep: resource.MustParse("-" + adjRes.Step),
			stepPercent:  adjRes.StepPercent,
			ratio:        1,
		})
	}
	adjAction.adjustResources = adjustResources
	adjAction.adjResKind = len(adjustResources)

	return adjAction
}

// ActionType return current action type
func (a *AdjustResourceAction) ActionType() ActionType {
	return Adjust
}

// DoAction handle check result
func (a *AdjustResourceAction) DoAction(conflicting bool, data interface{}) (*ActionResult, error) {
	var ac = &ActionResult{
		UnscheduleMap:   make(map[string]bool),
		AdjustResources: make(map[v1.ResourceName]ActionResource),
		Messages:        []string{data.(string)},
	}

	switch a.adjResKind {
	case 0:
	case 1:
		aRes := generateActionResource(a.adjustResources[0], conflicting)
		ac.AdjustResources[a.resource] = *aRes
	default:
		if a.op == opLoop {
			index := a.adjIndex
			a.adjIndex = (a.adjIndex + 1) % len(a.adjustResources)
			adjRes := a.adjustResources[index]
			aRes := generateActionResource(adjRes, conflicting)
			// distinguish when there are multi different action resources
			ac.AdjustResources[a.resource+"_"+adjRes.resource] = *aRes
		} else {
			for _, adjRes := range a.adjustResources {
				aRes := generateActionResource(adjRes, conflicting)
				// distinguish when there are multi different action resources
				ac.AdjustResources[a.resource+"_"+adjRes.resource] = *aRes
			}
		}
	}

	return ac, nil
}

// ResourceName returns checked resource name
func (a *AdjustResourceAction) ResourceName() v1.ResourceName {
	return a.resource
}

// InitSelfHandler init resource adjust handler
func (a *AdjustResourceAction) InitSelfHandler(resource v1.ResourceName, handler AdjustSelfDefineFunc) {
	for _, adjRes := range a.adjustResources {
		if adjRes.resource == resource {
			adjRes.actionResourceHandler = handler
			break
		}
	}
}

func generateActionResource(adjRes *adjustResource, conflicting bool) *ActionResource {
	// self-defined handler to get action resource
	if adjRes.actionResourceHandler != nil {
		selfResource := adjRes.actionResourceHandler(conflicting)
		if selfResource != nil {
			return selfResource
		}
	}

	var aRes = &ActionResource{
		Name:             adjRes.resource,
		ConflictQuantity: make(map[ActionFormula]resource.Quantity),
		Conflicting:      conflicting,
	}

	if !adjRes.step.IsZero() {
		aRes.ConflictQuantity[FormulaStep] = adjRes.step
		if conflicting {
			aRes.ConflictQuantity[FormulaStep] = adjRes.negativeStep
		}
	} else if adjRes.stepPercent != nil {
		if conflicting {
			adjRes.ratio = float64Round(adjRes.ratio * (1 - *adjRes.stepPercent))
		} else {
			adjRes.ratio = float64Round(adjRes.ratio / (1 - *adjRes.stepPercent))
			if adjRes.ratio > 1 {
				adjRes.ratio = 1
			}
		}
		// negative value
		r := -float64Round(1-adjRes.ratio) * 1000
		aRes.ConflictQuantity[FormulaPercent] = *resource.NewMilliQuantity(int64(r), resource.DecimalSI)
	}

	return aRes
}
