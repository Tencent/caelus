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
	"strings"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
	"github.com/tencent/caelus/pkg/caelus/util/runtime/docker"

	dockertypes "github.com/docker/engine-api/types"
	"gotest.tools/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	rootFsOfflineSize    = uint64(1024)
	rootFsAnnotationSize = uint64(1024000)

	containerNoId      = ""
	containerNoRunning = "db6088ead49a2247ff4f28b1d97f58859d91a41c93a979195220d64ea289c6a9"
	containerRunning   = "ebd4529dce0f4646a1d5e17259aae222500b2dca00852a51f16efad22fa56e16"
	rootFsPath         = "/var/lib/docker/overlay2/03099affb6d1595e2dddaf22224d0e6b8c911b8d112" +
		"3aae613d98dd2b4131a5e/merged"
)

type rootFsTestData struct {
	describe         string
	isOffline        bool
	hasQuota         bool
	hostNamespace    bool
	containerRunning bool
	pod              *v1.Pod
}

// TestRootFsQuotaManager_GetVolumes test if checking root fs volumes is ok
func TestRootFsQuotaManager_GetVolumes(t *testing.T) {
	containerNoRunningPodStatus := v1.PodStatus{
		ContainerStatuses: []v1.ContainerStatus{
			{
				ContainerID: "docker://" + containerNoId,
			},
			{
				ContainerID: "docker://" + containerNoRunning,
			},
			{
				ContainerID: "docker://" + containerRunning,
			},
		},
	}
	podStatus := v1.PodStatus{
		ContainerStatuses: []v1.ContainerStatus{
			{
				ContainerID: "docker://" + containerNoId,
			},
			{
				ContainerID: "docker://" + containerRunning,
			},
		},
	}

	testCases := []rootFsTestData{
		{
			describe:         "containerNotRunning",
			isOffline:        false,
			hasQuota:         true,
			hostNamespace:    true,
			containerRunning: false,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "rootfs-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: containerNoRunningPodStatus,
			},
		},
		{
			describe:         "onlinePodNoQuotaHostNamespace",
			isOffline:        false,
			hasQuota:         false,
			hostNamespace:    true,
			containerRunning: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					UID:         "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: podStatus,
			},
		},
		{
			describe:         "onlinePodHasQuotaHostNamespace",
			isOffline:        false,
			hasQuota:         true,
			hostNamespace:    true,
			containerRunning: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "rootfs-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: podStatus,
			},
		},
		{
			describe:         "onlinePodHasQuotaNoHostNamespace",
			isOffline:        false,
			hasQuota:         true,
			hostNamespace:    false,
			containerRunning: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "rootfs-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: podStatus,
			},
		},
		{
			describe:         "offlinePodNoQuotaHostNamespace",
			isOffline:        true,
			hasQuota:         false,
			hostNamespace:    true,
			containerRunning: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey: appclass.AnnotationOfflineValue,
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: podStatus,
			},
		},
		{
			describe:         "offlinePodHasQuotaHostNamespace",
			isOffline:        true,
			hasQuota:         true,
			hostNamespace:    true,
			containerRunning: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey:                  appclass.AnnotationOfflineValue,
						types.PodAnnotationPrefix + "rootfs-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: podStatus,
			},
		},
		{
			describe:         "offlinePodHasQuotaNotHostNamespace",
			isOffline:        true,
			hasQuota:         true,
			hostNamespace:    false,
			containerRunning: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey:                  appclass.AnnotationOfflineValue,
						types.PodAnnotationPrefix + "rootfs-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Status: podStatus,
			},
		},
	}

	for _, tc := range testCases {
		util.InHostNamespace = tc.hostNamespace
		t.Run(tc.describe, rootFsWrapperFunc(&tc))
	}
}

func rootFsWrapperFunc(tc *rootFsTestData) func(t *testing.T) {
	return func(t *testing.T) {
		containers := map[string]*dockertypes.ContainerJSON{
			containerRunning: {
				ContainerJSONBase: &dockertypes.ContainerJSONBase{
					State: &dockertypes.ContainerState{
						Status:  "running",
						Running: true,
					},
					GraphDriver: dockertypes.GraphDriverData{
						Data: map[string]string{
							"MergedDir": rootFsPath,
							"UpperDir": "/var/lib/docker/overlay2/" +
								"03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/diff",
							"WorkDir": "/var/lib/docker/overlay2/" +
								"03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/work",
						},
					},
				},
			},
		}
		if !tc.containerRunning {
			containers[containerNoRunning] = &dockertypes.ContainerJSON{
				ContainerJSONBase: &dockertypes.ContainerJSONBase{
					State: &dockertypes.ContainerState{
						Status:  "pending",
						Running: false,
					},
					GraphDriver: dockertypes.GraphDriverData{
						Data: map[string]string{
							"MergedDir": "/var/lib/docker/overlay2/" +
								"03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/merged",
							"UpperDir": "/var/lib/docker/overlay2/" +
								"03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/diff",
							"WorkDir": "/var/lib/docker/overlay2/" +
								"03099affb6d1595e2dddaf22224d0e6b8c911b8d1123aae613d98dd2b4131a5e/work",
						},
					},
				},
			}
		}

		r := &rootFsQuotaManager{
			runtimeClient: dockerclient.NewFakeDockerClient(containers),
			offlineSize: &types.DiskQuotaSize{
				Quota:  emptyDirOfflineSize,
				Inodes: 1000,
			},
		}
		paths, err := r.GetVolumes(tc.pod)
		if !tc.containerRunning {
			assert.Equal(t, err == nil, false)
			assert.Equal(t, strings.Contains(err.Error(), "root path is nil"), true)
			return
		}
		assert.NilError(t, err)

		expectPaths := make(map[string]*types.PathInfo)

		hostpath := rootFsPath
		if !tc.hostNamespace {
			hostpath = path.Join(types.RootFS, hostpath)
		}
		hostpath = strings.TrimSuffix(hostpath, "/merged")

		if tc.hasQuota {
			expectPaths[containerRunning[0:12]] = &types.PathInfo{
				Path: hostpath,
				Size: &types.DiskQuotaSize{
					Quota: rootFsAnnotationSize,
				},
			}
		} else if tc.isOffline {
			expectPaths[containerRunning[0:12]] = &types.PathInfo{
				Path: hostpath,
				Size: &types.DiskQuotaSize{
					Quota: rootFsOfflineSize,
				},
			}
		}

		rootFsAssertEqual(t, paths, expectPaths)
	}
}

func rootFsAssertEqual(t *testing.T, paths, expectPaths map[string]*types.PathInfo) {
	assert.Equal(t, len(paths), len(expectPaths))

	for name, path := range paths {
		expectPath, ok := expectPaths[name]
		assert.Equal(t, ok, true)
		assert.Equal(t, path.Path, expectPath.Path)
		assert.Equal(t, path.Size.Quota, expectPath.Size.Quota)
	}
}
