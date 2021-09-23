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
	"path"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"gotest.tools/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	emptyDirOfflineSize    = uint64(1024)
	emptyDirAnnotationSize = uint64(1024000)
	emptyDirLimitSize      = uint64(1024000000)
	emptyDirMemLimitName   = "memory-limit"
	emptyDirNoLimitName    = "no-limit"
	emptyDirLimitName      = "has-limit"
)

type emptyDirTestData struct {
	describe      string
	isOffline     bool
	hasQuota      bool
	hostNamespace bool
	pod           *v1.Pod
}

// TestEmptyDirQuotaManager_GetVolumes test if checking empty dir volumes is ok
func TestEmptyDirQuotaManager_GetVolumes(t *testing.T) {
	podSpec := v1.PodSpec{
		Volumes: []v1.Volume{
			{
				Name: emptyDirMemLimitName,
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{
						Medium:    v1.StorageMediumMemory,
						SizeLimit: resource.NewQuantity(int64(emptyDirLimitSize), resource.DecimalSI),
					},
				},
			},
			{
				Name: emptyDirNoLimitName,
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: emptyDirLimitName,
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{
						SizeLimit: resource.NewQuantity(int64(emptyDirLimitSize), resource.DecimalSI),
					},
				},
			},
		},
	}

	testCases := []emptyDirTestData{
		{
			describe:      "onlinePodNoQuotaHostNamespace",
			isOffline:     false,
			hasQuota:      false,
			hostNamespace: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					UID:         "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
		{
			describe:      "onlinePodHasQuotaHostNamespace",
			isOffline:     false,
			hasQuota:      true,
			hostNamespace: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "emptydir-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
		{
			describe:      "onlinePodHasQuotaNoHostNamespace",
			isOffline:     false,
			hasQuota:      true,
			hostNamespace: false,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "emptydir-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
		{
			describe:      "offlinePodNoQuotaHostNamespace",
			isOffline:     true,
			hasQuota:      false,
			hostNamespace: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey: appclass.AnnotationOfflineValue,
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
		{
			describe:      "offlinePodHasQuotaHostNamespace",
			isOffline:     true,
			hasQuota:      true,
			hostNamespace: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey:                    appclass.AnnotationOfflineValue,
						types.PodAnnotationPrefix + "emptydir-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
		{
			describe:      "offlinePodHasQuotaNotHostNamespace",
			isOffline:     true,
			hasQuota:      true,
			hostNamespace: false,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey:                    appclass.AnnotationOfflineValue,
						types.PodAnnotationPrefix + "emptydir-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
	}

	for _, tc := range testCases {
		util.InHostNamespace = tc.hostNamespace
		t.Run(tc.describe, emptyDirWrapperFunc(tc.pod, tc.isOffline, tc.hasQuota, tc.hostNamespace))
	}
}

func emptyDirWrapperFunc(pod *v1.Pod, isOffline, hasQuota, hostNamespace bool) func(t *testing.T) {
	return func(t *testing.T) {
		emptyManager := &emptyDirQuotaManager{
			offlineSize: &types.DiskQuotaSize{
				Quota:  emptyDirOfflineSize,
				Inodes: 1000,
			},
			kubeletRootDir: "/data",
		}
		paths, err := emptyManager.GetVolumes(pod)
		assert.NilError(t, err)

		expectPaths := make(map[string]*types.PathInfo)

		noLimitEmptyDirPath := path.Join(emptyManager.kubeletRootDir, "pods", string(pod.UID), "volumes",
			"kubernetes.io~empty-dir", emptyDirNoLimitName)
		if !hostNamespace {
			noLimitEmptyDirPath = path.Join(types.RootFS, noLimitEmptyDirPath)
		}
		if hasQuota {
			expectPaths[emptyDirNoLimitName] = &types.PathInfo{
				Path: noLimitEmptyDirPath,
				Size: &types.DiskQuotaSize{
					Quota: emptyDirAnnotationSize,
				},
			}
		} else if isOffline {
			expectPaths[emptyDirNoLimitName] = &types.PathInfo{
				Path: noLimitEmptyDirPath,
				Size: &types.DiskQuotaSize{
					Quota: emptyDirOfflineSize,
				},
			}
		}

		limitEmptyDirPath := path.Join(emptyManager.kubeletRootDir, "pods", string(pod.UID), "volumes",
			"kubernetes.io~empty-dir", emptyDirLimitName)
		if !hostNamespace {
			limitEmptyDirPath = path.Join(types.RootFS, limitEmptyDirPath)
		}
		expectPaths[emptyDirLimitName] = &types.PathInfo{
			Path: limitEmptyDirPath,
			Size: &types.DiskQuotaSize{
				Quota: emptyDirLimitSize,
			},
		}

		emptyDirAssertEqual(t, paths, expectPaths)
	}
}

func emptyDirAssertEqual(t *testing.T, paths, expectPaths map[string]*types.PathInfo) {
	assert.Equal(t, len(paths), len(expectPaths))

	for name, path := range paths {
		expectPath, ok := expectPaths[name]
		assert.Equal(t, ok, true)
		assert.Equal(t, path.Path, expectPath.Path)
		assert.Equal(t, path.Size.Quota, expectPath.Size.Quota)
	}
}
