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
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"gotest.tools/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	hostPathOfflineSize    = uint64(1024)
	hostPathAnnotationSize = uint64(1024000)

	hostPathNotExistedName = "path-not-existed"
	hostPathIsFileName     = "path-is-File"
	hostPathIsRootDir      = "path-is-root-dir"
	hostPathIsMountPoint   = "path-is-mount-point"
	hostPathNormal         = "path-is-normal"

	hostPathTestPaths = map[string]struct {
		path       string
		isFile     bool
		needCreate bool
	}{
		"var":        {path: "/var", isFile: false, needCreate: false},
		"cpuinfo":    {path: "/proc/cpuinfo", isFile: true, needCreate: false},
		"root":       {path: "/", isFile: false, needCreate: false},
		"test":       {path: "/hostpath-test", isFile: false, needCreate: true},
		"nonExisted": {path: "/aaa", isFile: false, needCreate: false},
	}
)

type hostPathTestData struct {
	describe      string
	isOffline     bool
	hasQuota      bool
	hostNamespace bool
	err           error
	pod           *v1.Pod
}

// TestHostPathQuotaManager_GetVolumes test if checking host path volumes is ok
func TestHostPathQuotaManager_GetVolumes(t *testing.T) {
	for _, v := range hostPathTestPaths {
		if v.needCreate {
			err := os.MkdirAll(v.path, 0777)
			if err != nil {
				t.Skipf("create path %s err: %v", v.path, err)
			}
		}

	}
	defer func() {
		for _, v := range hostPathTestPaths {
			if v.needCreate {
				os.RemoveAll(v.path)
			}
		}
	}()

	testCases := generateHostPathGetVolumesTestCases()
	for _, tc := range testCases {
		util.InHostNamespace = tc.hostNamespace
		t.Run(tc.describe, hostPathWrapperFunc(tc))
	}
}

func generateHostPathGetVolumesTestCases() []hostPathTestData {
	podSpec, hasNoExistedPathPodSpec := generateHostPathGetVolumesPodSpec()
	testCases := []hostPathTestData{
		{
			describe:      "hostPathNotExisted",
			isOffline:     false,
			hasQuota:      true,
			hostNamespace: true,
			err:           fmt.Errorf("no such file or directory"),
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "hostpath-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: hasNoExistedPathPodSpec,
			},
		},
		{
			describe:      "onlinePodNoQuotaHostNamespace",
			isOffline:     false,
			hasQuota:      false,
			hostNamespace: true,
			err:           nil,
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
			err:           nil,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						types.PodAnnotationPrefix + "hostpath-diskquota": "quota=1024000,inodes=10000",
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
			err:           nil,
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
			err:           nil,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appclass.AnnotationOfflineKey:                    appclass.AnnotationOfflineValue,
						types.PodAnnotationPrefix + "hostpath-diskquota": "quota=1024000,inodes=10000",
					},
					UID: "01780108-c5dd-457e-b99a-dade618bd1a1",
				},
				Spec: podSpec,
			},
		},
	}

	return testCases
}

func generateHostPathGetVolumesPodSpec() (podSpec, hasNoExistedPathPodSpec v1.PodSpec) {
	hostPathType := new(v1.HostPathType)
	*hostPathType = v1.HostPathType(string(v1.HostPathFile))
	podSpec = v1.PodSpec{
		Volumes: []v1.Volume{
			{
				Name: hostPathIsFileName,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: hostPathTestPaths["cpuinfo"].path,
						Type: hostPathType,
					},
				},
			},
			{
				Name: hostPathIsRootDir,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: hostPathTestPaths["var"].path,
					},
				},
			},
			{
				Name: hostPathIsMountPoint,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: hostPathTestPaths["root"].path,
					},
				},
			},
			{
				Name: hostPathNormal,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: hostPathTestPaths["test"].path,
					},
				},
			},
		},
	}

	hasNoExistedPathPodSpec = v1.PodSpec{
		Volumes: []v1.Volume{
			{
				Name: hostPathNotExistedName,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: hostPathTestPaths["nonExisted"].path,
					},
				},
			},
		},
	}

	return podSpec, hasNoExistedPathPodSpec
}

func hostPathWrapperFunc(tc hostPathTestData) func(t *testing.T) {
	return func(t *testing.T) {
		h := &hostPathQuotaManager{
			offlineSize: &types.DiskQuotaSize{
				Quota:  emptyDirOfflineSize,
				Inodes: 1000,
			},
		}
		paths, err := h.GetVolumes(tc.pod)
		if tc.err == nil {
			assert.NilError(t, err)
		} else {
			assert.Equal(t, err == nil, false)
			assert.Equal(t, strings.Contains(err.Error(), tc.err.Error()), true)
			return
		}

		expectPaths := make(map[string]*types.PathInfo)

		hostpath := hostPathTestPaths["test"].path
		if !tc.hostNamespace {
			hostpath = path.Join(types.RootFS, hostpath)
		}

		if tc.hasQuota {
			expectPaths[hostPathNormal] = &types.PathInfo{
				Path: hostpath,
				Size: &types.DiskQuotaSize{
					Quota: hostPathAnnotationSize,
				},
			}
		} else if tc.isOffline {
			expectPaths[hostPathNormal] = &types.PathInfo{
				Path: hostpath,
				Size: &types.DiskQuotaSize{
					Quota: hostPathOfflineSize,
				},
			}
		}

		hostPathAssertEqual(t, paths, expectPaths)
	}
}

func hostPathAssertEqual(t *testing.T, paths, expectPaths map[string]*types.PathInfo) {
	assert.Equal(t, len(paths), len(expectPaths))

	for name, path := range paths {
		expectPath, ok := expectPaths[name]
		assert.Equal(t, ok, true)
		assert.Equal(t, path.Path, expectPath.Path)
		assert.Equal(t, path.Size.Quota, expectPath.Size.Quota)
	}
}
