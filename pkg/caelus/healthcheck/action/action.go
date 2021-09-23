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
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
	api "k8s.io/apimachinery/pkg/api/resource"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type ActionType string
type ActionFormula string

const (
	Adjust   ActionType = "adjust"
	Schedule ActionType = "schedule"
	Evict    ActionType = "evict"
	Log      ActionType = "log"

	// FormulaStep means removed step by step, and it's negative value if the resource is conflicting
	FormulaStep ActionFormula = "step"
	// FormulaTotal means removed by total, and it's negative value if the resource is conflicting
	FormulaTotal ActionFormula = "total"
	// FormulaPercent means removed by percent, and it's negative value if the resource is conflicting
	FormulaPercent ActionFormula = "percent"
)

var (
	allActionFormula = []ActionFormula{FormulaStep, FormulaTotal, FormulaPercent}
	zeroQuantity     = api.MustParse("0")
)

// Action is the interface of actions when rules match
type Action interface {
	ActionType() ActionType
	DoAction(conflicting bool, data interface{}) (*ActionResult, error)
}

// ActionResource records resource quantity need to remove or add.
type ActionResource struct {
	Name             v1.ResourceName
	Conflicting      bool
	ConflictQuantity map[ActionFormula]api.Quantity
}

// MergeByLittle merge two ActionResource into one, and choose the little conflicting value
func (a ActionResource) MergeByLittle(b ActionResource) ActionResource {
	c := ActionResource{
		Name:             a.Name,
		Conflicting:      a.Conflicting || b.Conflicting,
		ConflictQuantity: make(map[ActionFormula]api.Quantity),
	}

	for _, k := range allActionFormula {
		v1, ok1 := a.ConflictQuantity[k]
		v2, ok2 := b.ConflictQuantity[k]

		if ok1 && ok2 {
			if v1.Cmp(v2) < 0 {
				c.ConflictQuantity[k] = v1.DeepCopy()
			} else {
				c.ConflictQuantity[k] = v2.DeepCopy()
			}
		} else if ok1 {
			c.ConflictQuantity[k] = v1.DeepCopy()
		} else if ok2 {
			c.ConflictQuantity[k] = v2.DeepCopy()
		}
	}
	return c
}

// DeepCopy copy ActionResource
func (a ActionResource) DeepCopy() ActionResource {
	b := ActionResource{
		Name:             a.Name,
		Conflicting:      a.Conflicting,
		ConflictQuantity: make(map[ActionFormula]api.Quantity),
	}

	for k, v := range a.ConflictQuantity {
		b.ConflictQuantity[k] = v.DeepCopy()
	}

	return b
}

// IsNegative returns if conflicting or conflict resource is negative
func (a ActionResource) IsNegative() bool {
	if a.Conflicting {
		return true
	}
	for _, q := range a.ConflictQuantity {
		if q.Cmp(zeroQuantity) < 0 {
			return true
		}
	}

	return false
}

// Equal check if the two actionResource is equal
func (a *ActionResource) Equal(b *ActionResource) bool {
	if len(a.Name) != 0 && len(b.Name) != 0 {
		if a.Name != b.Name {
			return false
		}
	}

	if a.Conflicting != b.Conflicting {
		return false
	}

	if len(a.ConflictQuantity) != len(b.ConflictQuantity) {
		return false
	}
	for k, v := range a.ConflictQuantity {
		vv, ok := b.ConflictQuantity[k]
		if !ok {
			return false
		}
		if !v.Equal(vv) {
			return false
		}
	}

	return true
}

// ActionResult is the result of actions
type ActionResult struct {
	UnscheduleMap   map[string]bool
	EvictPods       []k8stypes.NamespacedName
	AdjustResources map[v1.ResourceName]ActionResource
	SyncEvent       bool
	Messages        []string
}

// Merge merge the other action result
func (ar *ActionResult) Merge(other *ActionResult) {
	ar.SyncEvent = ar.SyncEvent || other.SyncEvent

	// merge message list
	if len(other.Messages) != 0 {
		msgs := sets.NewString()
		for _, m := range ar.Messages {
			msgs.Insert(m)
		}
		for _, m := range other.Messages {
			if !msgs.Has(m) {
				msgs.Insert(m)
				ar.Messages = append(ar.Messages, m)
			}
		}
	}

	// merge un-schedule state map
	for k, v := range other.UnscheduleMap {
		if v {
			ar.UnscheduleMap[k] = true
			continue
		}
		if _, exist := ar.UnscheduleMap[k]; !exist {
			ar.UnscheduleMap[k] = v
		}
	}

	// merge evict pods
	if len(other.EvictPods) != 0 {
		pods := sets.NewString()
		for _, pod := range ar.EvictPods {
			key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			pods.Insert(key)
		}
		for _, pod := range other.EvictPods {
			key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			if !pods.Has(key) {
				pods.Insert(key)
				ar.EvictPods = append(ar.EvictPods, pod)
			}
		}
	}

	// merge adjust resource list
	for k, v := range other.AdjustResources {
		v2, exist := ar.AdjustResources[k]
		if !exist {
			ar.AdjustResources[k] = v
		} else {
			ar.AdjustResources[k] = v2.MergeByLittle(v)
		}
	}
	return
}

// String implements Stringer interface
func (ar *ActionResult) String() string {
	var msg []string
	if len(ar.UnscheduleMap) != 0 {
		msg = append(msg, fmt.Sprintf("unschedule=%v", ar.UnscheduleMap))
	}
	if len(ar.EvictPods) != 0 {
		msg = append(msg, fmt.Sprintf("EvictPods: %v", ar.EvictPods))
	}
	if len(ar.AdjustResources) != 0 {
		msg = append(msg, fmt.Sprintf("AdjustResources: %v", ar.AdjustResources))
	}
	msg = append(msg, fmt.Sprintf("SyncEvent: %v", ar.SyncEvent))
	msg = append(msg, fmt.Sprintf("Message: %v", ar.Messages))
	return strings.Join(msg, ", ")
}
