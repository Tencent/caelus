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

package yarn

import (
	global "github.com/tencent/caelus/pkg/types"
)

type byAmAndTime []global.NMContainer

// Len output the length of container list
func (s byAmAndTime) Len() int {
	return len(s)
}

// Swap implement the swap function
func (s byAmAndTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less implement compare function
func (s byAmAndTime) Less(i, j int) bool {
	if s[i].IsAM {
		if !s[j].IsAM {
			return false
		}
	} else if s[j].IsAM {
		return true
	}
	//TODO consider time weight and resource weight together
	return s[i].StartTime.After(s[j].StartTime)
}

type byAmAndCPU []global.NMContainer

// Len output the length of container list
func (s byAmAndCPU) Len() int {
	return len(s)
}

// Swap implement swap function
func (s byAmAndCPU) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less implement compare function
func (s byAmAndCPU) Less(i, j int) bool {
	if s[i].IsAM {
		if !s[j].IsAM {
			return false
		}
	} else if s[j].IsAM {
		return true
	}
	if s[i].UsedVCores == s[j].UsedVCores {
		return s[i].StartTime.After(s[j].StartTime)
	} else {
		return s[i].UsedVCores > s[j].UsedVCores
	}
}

type byAmAndMemory []global.NMContainer

// Len output the length of container list
func (s byAmAndMemory) Len() int {
	return len(s)
}

// Swap implement swap function
func (s byAmAndMemory) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less implement compare function
func (s byAmAndMemory) Less(i, j int) bool {
	if s[i].IsAM {
		if !s[j].IsAM {
			return false
		}
	} else if s[j].IsAM {
		return true
	}
	if s[i].UsedMemoryMB == s[j].UsedMemoryMB {
		return s[i].StartTime.After(s[j].StartTime)
	} else {
		return s[i].UsedMemoryMB > s[j].UsedMemoryMB
	}
}
