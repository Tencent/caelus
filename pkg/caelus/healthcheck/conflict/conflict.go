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

package conflict

import (
	"sync"

	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"

	"k8s.io/api/core/v1"
	api "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

var zeroQ = api.MustParse("0")

// Manager is the interface to manage conflict
type Manager interface {
	// CheckAndSubConflictResource removes conflicted quantity from resources given in parameters
	CheckAndSubConflictResource(predictList v1.ResourceList) (map[v1.ResourceName]bool, error)
	UpdateConflictList(conflictList map[v1.ResourceName]action.ActionResource) (bool, error)
}

type manager struct {
	conflictingLock sync.RWMutex
	// negative value if the resource in conflicting state
	conflictingResources map[v1.ResourceName]action.ActionResource
}

// NewConflictManager creates a new conflict manager
func NewConflictManager() Manager {
	return &manager{
		conflictingResources: make(map[v1.ResourceName]action.ActionResource),
	}
}

// UpdateConflictList update conflict resource
func (c *manager) UpdateConflictList(conflictList map[v1.ResourceName]action.ActionResource) (bool, error) {
	changed := false

	c.conflictingLock.Lock()
	defer c.conflictingLock.Unlock()
	for k, v := range conflictList {
		originV, ok := c.conflictingResources[k]
		if !ok {
			// just add negative resource, that is in conflicting state
			if v.IsNegative() {
				c.conflictingResources[k] = v.DeepCopy()
				changed = true
			}
			continue
		}

		originV.Conflicting = v.Conflicting
		for formula, q := range v.ConflictQuantity {
			originQ, ok := originV.ConflictQuantity[formula]
			switch formula {
			case action.FormulaStep:
				if q.Cmp(zeroQ) < 0 {
					changed = true
				}

				if !ok {
					// if q is non-negative value, it will be deleted later
					originQ = q
				} else {
					if !originQ.IsZero() {
						// originQ is a non-positive value, so if it is not zero, it will be added to a new value
						changed = true
					}
					originQ.Add(q)
				}

				// conflicting resource with FormulaTotal will be recovered by FormulaStep
				if originTotalQ, okTotal := originV.ConflictQuantity[action.FormulaTotal]; okTotal &&
					q.Cmp(zeroQ) > 0 {
					originTotalQ.Add(q)
					changed = true
					if originTotalQ.Cmp(zeroQ) >= 0 {
						klog.V(2).Infof("conflicting resource with FormulaTotal is recovered by Step")
						delete(originV.ConflictQuantity, action.FormulaTotal)
					} else {
						originV.ConflictQuantity[action.FormulaTotal] = originTotalQ
					}
				}
			case action.FormulaTotal:
				fallthrough
			case action.FormulaPercent:
				if !ok {
					originQ = q
					changed = true
				} else if originQ.Cmp(q) != 0 {
					// always replace the new value
					originQ = q
					changed = true
				}
			}

			if originQ.Cmp(zeroQ) >= 0 {
				delete(originV.ConflictQuantity, formula)
				continue
			}
			originV.ConflictQuantity[formula] = originQ
		}
		if len(originV.ConflictQuantity) == 0 {
			delete(c.conflictingResources, k)
			continue
		}
		c.conflictingResources[k] = originV
	}
	if changed {
		klog.Warningf("conflicting resource changing to: %+v", c.conflictingResources)
	}
	return changed, nil
}

// CheckAndSubConflictResource sub conflicting resource
func (c *manager) CheckAndSubConflictResource(predictList v1.ResourceList) (map[v1.ResourceName]bool, error) {
	conflictedRes := make(map[v1.ResourceName]bool)

	mergedList, mergedConflict := c.mergeConflictResource(predictList)
	for resName, q := range mergedList {
		originQt, _ := predictList[resName]
		originQt.Add(q)
		if originQt.Cmp(zeroQ) < 0 {
			klog.Warningf("negative resource value for %s, just set it zero", resName)
			originQt = zeroQ
		}
		predictList[resName] = originQt
		conflictedRes[resName] = mergedConflict[resName]
	}

	return conflictedRes, nil
}

func (c *manager) mergeConflictResource(predictList v1.ResourceList) (
	mergedResourceList map[v1.ResourceName]api.Quantity, mergedConflicting map[v1.ResourceName]bool) {
	mergedResourceList = make(map[v1.ResourceName]api.Quantity)
	mergedConflicting = make(map[v1.ResourceName]bool)

	c.conflictingLock.Lock()
	defer c.conflictingLock.Unlock()
	for k, rq := range c.conflictingResources {
		resName := k
		if len(rq.Name) != 0 {
			// using new resource name
			klog.V(3).Infof("translating resource: %s - %s", k, rq.Name)
			resName = rq.Name
		}

		// no need to check missed resource
		originQt, ok := predictList[resName]
		if !ok {
			continue
		}

		// get current resource smaller quantity among different formulas
		var currentQ = zeroQ
		for formula, v := range rq.ConflictQuantity {
			if v.IsZero() {
				continue
			}

			switch formula {
			case action.FormulaStep:
				fallthrough
			case action.FormulaTotal:
				// reset conflict quantity to current predict quantity
				v1 := v.DeepCopy()
				v1.Add(originQt)
				if v1.Cmp(zeroQ) < 0 {
					originQt1 := originQt.DeepCopy()
					originQt1.Neg()
					c.conflictingResources[k].ConflictQuantity[formula] = originQt1
					klog.Warningf("conflicting resource changing to: %+v", c.conflictingResources)
				}

				if v.Cmp(currentQ) < 0 {
					currentQ = v.DeepCopy()
				}
			case action.FormulaPercent:
				millV := int64(float64(originQt.MilliValue()) * float64(v.MilliValue()) / 1000)
				newV := *api.NewMilliQuantity(millV, api.DecimalSI)
				if newV.Cmp(currentQ) < 0 {
					currentQ = newV.DeepCopy()
				}
			}
		}
		if currentQ.IsZero() {
			continue
		}

		// compare with old conflict resource
		if value, ok := mergedResourceList[resName]; !ok {
			mergedResourceList[resName] = currentQ
			mergedConflicting[resName] = rq.Conflicting
		} else {
			// choose the smaller one
			if currentQ.Cmp(value) < 0 {
				mergedResourceList[resName] = currentQ
				klog.V(2).Infof("conflict resource %s find smaller from %s, and updat to: %v -> %v",
					resName, k, value, currentQ)
			}
			// update if the new resource is in conflicting state
			if rq.Conflicting {
				mergedConflicting[resName] = true
			}
		}
	}

	return mergedResourceList, mergedConflicting
}
