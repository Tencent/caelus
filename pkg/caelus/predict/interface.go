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

package predict

import (
	"k8s.io/api/core/v1"
)

// Interface is the predict interface
type Interface interface {
	// GetAllocatableForBatch return allocatable resources for offline jobs
	GetAllocatableForBatch() v1.ResourceList
	// GetReservedResource return reserved resource quantity
	GetReservedResource() v1.ResourceList
	// Run starts predict
	Run(stop <-chan struct{})
	// module name
	Name() string
}

// Predictor predicts resources used by online and system pods
type Predictor interface {
	// Predict predicts resources used by online and system pods
	Predict() v1.ResourceList
	// AddSample starts predictor
	AddSample(stop <-chan struct{})
}
