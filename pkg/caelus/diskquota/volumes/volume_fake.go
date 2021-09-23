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

package volume

import (
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
	apiType "k8s.io/apimachinery/pkg/types"
)

var (
	FakeVolumeQuotaManagerName types.VolumeType = "fakeVolumeQuotaManager"
)

type fakeVolumeManager struct {
	pathInfos map[apiType.UID]map[string]*types.PathInfo
}

// NewFakeVolumeQuotaManager creates fake volume quota manager instance
func NewFakeVolumeQuotaManager(pathInfos map[apiType.UID]map[string]*types.PathInfo) VolumeQuotaManager {
	return &fakeVolumeManager{
		pathInfos: pathInfos,
	}
}

// Name return volume name
func (f *fakeVolumeManager) Name() types.VolumeType {
	return FakeVolumeQuotaManagerName
}

// GetVolumes return paths, which need to set quota
func (f *fakeVolumeManager) GetVolumes(pod *v1.Pod) (map[string]*types.PathInfo, error) {
	return f.pathInfos[pod.UID], nil
}
