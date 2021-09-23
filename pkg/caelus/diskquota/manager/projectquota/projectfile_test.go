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

package projectquota

import (
	"os"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"gotest.tools/assert"
)

var (
	testProjectsPath = "/tmp/projects"
	testProjidPath   = "/tmp/projid"
	testRootfs       = "/rootfs/tmp"
)

// TestProjectFile test operating on project file
func TestProjectFile(t *testing.T) {
	projectsPath = testProjectsPath
	projidPath = testProjidPath
	t.Run("project file host namespace test", hostNamespaceTest)
	t.Run("project file non host namespace test", nonHostNamespaceTest)
}

func hostNamespaceTest(t *testing.T) {
	util.InHostNamespace = true
	defer func() {
		os.RemoveAll(projectsPath)
		os.RemoveAll(projidPath)
	}()

	idPaths := map[quotaID][]string{
		quotaID(1048576): {"/aa", "/aa1"},
		quotaID(1048577): {"/bb", "/bb1"},
		quotaID(1048578): {"/cc", "/cc1"},
	}
	idNames := map[quotaID]string{
		quotaID(1048576): quotaID(1048576).IdName(types.VolumeTypeRootFs.String()),
		quotaID(1048577): quotaID(1048577).IdName(types.VolumeTypeEmptyDir.String()),
		quotaID(1048578): quotaID(1048578).IdName(types.VolumeTypeHostPath.String()),
	}

	prjFile := NewProjectFile()
	err := prjFile.UpdateProjects(idPaths)
	assert.NilError(t, err)
	err = prjFile.UpdateProjIds(idNames)
	assert.NilError(t, err)
	newIdPaths, newIdNames, err := prjFile.DumpProjectIds()
	assert.NilError(t, err)
	idPathAssertEqutal(t, newIdPaths, idPaths)
	idNameAssertEqutal(t, newIdNames, idNames)
}

func nonHostNamespaceTest(t *testing.T) {
	util.InHostNamespace = false
	err := os.MkdirAll(testRootfs, 0777)
	if err != nil {
		t.Skipf("create path(%s) err: %v", testRootfs, err)
	}
	defer func() {
		os.RemoveAll(projectsPath)
		os.RemoveAll(projidPath)
		os.RemoveAll(testRootfs)
	}()

	idPaths := map[quotaID][]string{
		quotaID(1048576): {types.RootFS + "/aa"},
		quotaID(1048577): {types.RootFS + "/bb"},
		quotaID(1048578): {types.RootFS + "/cc"},
	}
	idNames := map[quotaID]string{
		quotaID(1048576): quotaID(1048576).IdName(types.VolumeTypeRootFs.String()),
		quotaID(1048577): quotaID(1048577).IdName(types.VolumeTypeEmptyDir.String()),
		quotaID(1048578): quotaID(1048578).IdName(types.VolumeTypeHostPath.String()),
	}

	prjFile := NewProjectFile()
	err = prjFile.UpdateProjects(idPaths)
	assert.NilError(t, err)
	err = prjFile.UpdateProjIds(idNames)
	assert.NilError(t, err)
	newIdPaths, newIdNames, err := prjFile.DumpProjectIds()
	assert.NilError(t, err)
	idPathAssertEqutal(t, newIdPaths, idPaths)
	idNameAssertEqutal(t, newIdNames, idNames)
}

func idPathAssertEqutal(t *testing.T, idPaths, expectIdPaths map[quotaID][]string) {
	assert.Equal(t, len(idPaths), len(expectIdPaths))
	for id, v := range idPaths {
		vv, ok := expectIdPaths[id]
		assert.Equal(t, ok, true)
		assert.DeepEqual(t, v, vv)
	}
}

func idNameAssertEqutal(t *testing.T, idPaths, expectIdPaths map[quotaID]string) {
	assert.Equal(t, len(idPaths), len(expectIdPaths))
	for id, v := range idPaths {
		vv, ok := expectIdPaths[id]
		assert.Equal(t, ok, true)
		assert.Equal(t, v, vv)
	}
}
