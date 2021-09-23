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

import "math"

type evictAction struct{}

// NewEvictAction get action instance to evict app
func NewEvictAction() Action {
	return &evictAction{}
}

// ActionType return current action type
func (e *evictAction) ActionType() ActionType {
	return Evict
}

// DoAction handle check result
func (e *evictAction) DoAction(conflicting bool, data interface{}) (*ActionResult, error) {
	return nil, nil
}

// https://github.com/golang/go/issues/11660, we need to limit 2 decimal
func float64Round(v float64) float64 {
	return math.Round(v*100) / 100
}
