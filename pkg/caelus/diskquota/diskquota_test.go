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

package diskquota

import (
	"testing"

	"github.com/tencent/caelus/pkg/caelus/diskquota/manager"
	"github.com/tencent/caelus/pkg/caelus/diskquota/volumes"
	"github.com/tencent/caelus/pkg/caelus/types"

	"gotest.tools/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiType "k8s.io/apimachinery/pkg/types"
)

var (
	testVolumeName = "volumeTest"
	testVolumePath = "/test"
	testQuota      = uint64(1024000000)
	testQuotaUsed  = uint64(1024)
	testPodUid     = apiType.UID("01780108-c5dd-457e-b99a-dade618bd1a1")
)

// TestDiskQuota_handlePodDiskQuota test handle pod disk quota function
func TestDiskQuota_handlePodDiskQuota(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
			UID:         testPodUid,
		},
	}
	inputPathInfos := map[apiType.UID]map[string]*types.PathInfo{
		testPodUid: {
			testVolumeName: {
				Path: testVolumePath,
				Size: &types.DiskQuotaSize{
					Quota:     testQuota,
					QuotaUsed: testQuotaUsed,
				},
			},
		},
	}

	expectPathInfos := map[types.VolumeType]*VolumeInfo{
		volume.FakeVolumeQuotaManagerName: {
			getPathsSuccess:  true,
			setQuotasSuccess: true,
			Paths: map[string]*PathInfoWrapper{
				testVolumeName: {
					setQuotaSuccess: true,
					PathInfo: types.PathInfo{
						Path: testVolumePath,
						Size: &types.DiskQuotaSize{
							Quota:     testQuota,
							QuotaUsed: testQuotaUsed,
						},
					},
				},
			},
		},
	}

	d := &diskQuota{
		quotaManager:        manager.NewFakeQuotaManager(),
		volumeQuotaManagers: []volume.VolumeQuotaManager{volume.NewFakeVolumeQuotaManager(inputPathInfos)},
		handedPods:          make(map[apiType.UID]*PodVolumes),
	}

	d.handlePodDiskQuota(pod)
	diskQuotaAssertEqual(t, d.handedPods[testPodUid].Volumes, expectPathInfos)
	// call again, the quota used will changed
	d.handlePodDiskQuota(pod)
	quotaUsed := expectPathInfos[volume.FakeVolumeQuotaManagerName].Paths[testVolumeName].PathInfo.Size.QuotaUsed
	expectPathInfos[volume.FakeVolumeQuotaManagerName].Paths[testVolumeName].PathInfo.Size.QuotaUsed = quotaUsed * manager.QuotaSizeTimes
	diskQuotaAssertEqual(t, d.handedPods[testPodUid].Volumes, expectPathInfos)
}

func diskQuotaAssertEqual(t *testing.T, pathInfos, expectPathInfos map[types.VolumeType]*VolumeInfo) {
	assert.Equal(t, len(pathInfos), len(expectPathInfos))

	for k, v := range pathInfos {
		vv, ok := expectPathInfos[k]
		assert.Equal(t, ok, true)
		assert.Equal(t, v.getPathsSuccess, vv.getPathsSuccess)
		assert.Equal(t, v.setQuotasSuccess, vv.setQuotasSuccess)
		assert.Equal(t, len(v.Paths), len(vv.Paths))
		for kk, p := range v.Paths {
			pp, ok := vv.Paths[kk]
			assert.Equal(t, ok, true)
			assert.Equal(t, p.setQuotaSuccess, pp.setQuotaSuccess)
			assert.Equal(t, p.Path, pp.Path)
			assert.Equal(t, p.Size.Quota, pp.Size.Quota)
			assert.Equal(t, p.Size.QuotaUsed, pp.Size.QuotaUsed)
		}
	}
}
