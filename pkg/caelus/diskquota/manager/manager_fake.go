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

package manager

import (
	"fmt"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	QuotaSizeTimes uint64 = 2
)

type fakeQuotaManager struct {
	quotas      map[string]*types.DiskQuotaSize
	volumeTypes map[string]types.VolumeType
}

// NewFakeQuotaManager new fake quota manager instance
func NewFakeQuotaManager() QuotaManager {
	return &fakeQuotaManager{
		quotas:      make(map[string]*types.DiskQuotaSize),
		volumeTypes: make(map[string]types.VolumeType),
	}
}

// GetQuota get disk quota for the target path
func (f *fakeQuotaManager) GetQuota(targetPath string) (*types.DiskQuotaSize, error) {
	quota, ok := f.quotas[targetPath]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	// twice during call
	quota.QuotaUsed = quota.QuotaUsed * QuotaSizeTimes
	return quota, nil
}

// SetQuota set disk quota for path. pathFlag shows which the path is, such as emptyDir or hostPath
func (f *fakeQuotaManager) SetQuota(targetPath string, pathFlag types.VolumeType,
	size *types.DiskQuotaSize, sharedInfo *types.SharedInfo) error {
	f.quotas[targetPath] = size
	f.volumeTypes[targetPath] = pathFlag

	return nil
}

// ClearQuota clears quota
func (f *fakeQuotaManager) ClearQuota(targetPath string) error {
	delete(f.quotas, targetPath)
	delete(f.volumeTypes, targetPath)

	return nil
}

// GetAllQuotaPath return all paths which has set quota, and classified by path flag
func (f *fakeQuotaManager) GetAllQuotaPath() map[types.VolumeType]sets.String {
	allPaths := make(map[types.VolumeType]sets.String)
	for p, t := range f.volumeTypes {
		paths, ok := allPaths[t]
		if !ok {
			paths = sets.NewString()
		}
		paths.Insert(p)
		allPaths[t] = paths
	}

	return allPaths
}
