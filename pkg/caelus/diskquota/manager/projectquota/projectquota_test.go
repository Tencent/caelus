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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/mountpoint"

	"golang.org/x/sys/unix"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

const (
	// 10MB
	testQuotaSize = 10 * 1024 * 1024
	imageSize     = 64 * 1024 * 1024
)

// TestDiskQuotaTest test operations on project quota
// How to create image is copied from github.com/docker/docker/daemon/graphdriver/quota/projectquota_test.go
func TestDiskQuotaTest(t *testing.T) {
	mkfs, err := exec.LookPath("mkfs.xfs")
	if err != nil {
		t.Skip("mkfs.xfs not found in PATH")
	}

	// create a sparse image
	imageFile, err := ioutil.TempFile("", "xfs-image")
	if err != nil {
		t.Fatal(err)
	}
	imageFileName := imageFile.Name()
	defer os.Remove(imageFileName)
	if _, err = imageFile.Seek(imageSize-1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = imageFile.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if err = imageFile.Close(); err != nil {
		t.Fatal(err)
	}

	// The reason for disabling these options is sometimes people run with a newer userspace
	// than kernelspace
	out, err := exec.Command(mkfs, "-m", "crc=0,finobt=0", imageFileName).CombinedOutput()
	if len(out) > 0 {
		t.Log(string(out))
	}
	if err != nil {
		t.Fatal(err)
	}

	t.Run("testBackingDevNotSupported", wrapMountTest(imageFileName, false,
		testFindOrCreateDevNoSupported))
	t.Run("testBackingDevSupported", wrapMountTest(imageFileName, true,
		testFindOrCreateDevSupported))
	t.Run("testProjectId", wrapMountTest(imageFileName, true, testFindOrCreateProjectId))
	t.Run("testSetQuota", wrapMountTest(imageFileName, true, testSetQuota))
	t.Run("testGetQuota", wrapMountTest(imageFileName, true, testGetQuota))
	t.Run("testClearQuota", wrapMountTest(imageFileName, true, testClearQuota))
}

func wrapMountTest(imageFileName string, enableQuota bool,
	testFunc func(t *testing.T, targetPath string)) func(*testing.T) {
	return func(t *testing.T) {
		mountpoint.MountsInitialized = false

		mountOptions := "loop"

		if enableQuota {
			mountOptions = mountOptions + ",prjquota"
		}

		mountPoint := "/xfs-mountPoint"
		err := os.MkdirAll(mountPoint, 0777)
		if err != nil {
			t.Skipf("creat xfs mount point directory err: %v", err)
		}
		defer os.RemoveAll(mountPoint)

		out, err := exec.Command("mount", "-o", mountOptions, imageFileName, mountPoint).CombinedOutput()
		if err != nil {
			_, err := os.Stat("/proc/fs/xfs")
			if os.IsNotExist(err) {
				t.Skip("no /proc/fs/xfs")
			}
		}

		assert.NilError(t, err, "mount failed: %s", out)

		defer func() {
			assert.NilError(t, unix.Unmount(mountPoint, 0))
		}()

		targetPath := path.Join(mountPoint, "innerPath")
		err = os.MkdirAll(targetPath, 0777)
		if err != nil {
			t.Fatalf("create path(%s) err: %v", targetPath, err)
		}
		defer os.RemoveAll(targetPath)

		testFunc(t, targetPath)
	}
}

func testFindOrCreateDevNoSupported(t *testing.T, targetPath string) {
	backingFsDev := testFindOrCreateDev(t, targetPath)
	assert.Check(t, !backingFsDev.supported)
}

func testFindOrCreateDevSupported(t *testing.T, targetPath string) {
	backingFsDev := testFindOrCreateDev(t, targetPath)
	assert.Check(t, backingFsDev.supported)
}

func testFindOrCreateDev(t *testing.T, targetPath string) *backingDev {
	p := mockProjectQuota([]quotaID{})
	backingFsDev, err := p.findOrCreateBackingDev(targetPath)
	assert.NilError(t, err)
	return backingFsDev
}

func testFindOrCreateProjectId(t *testing.T, targetPath string) {
	p := mockProjectQuota([]quotaID{quotaID(1048577), quotaID(1048578)})
	pid, isNew, err := p.findOrCreateProjectId(targetPath, types.VolumeTypeRootFs.String(),
		false, !persistToFile)
	assert.NilError(t, err)
	assert.Check(t, isNew)
	assert.Equal(t, pid, quotaID(1048579))

	// check again
	pid, isNew, err = p.findOrCreateProjectId(targetPath, types.VolumeTypeRootFs.String(),
		false, !persistToFile)
	assert.NilError(t, err)
	assert.Check(t, !isNew)
	assert.Equal(t, pid, quotaID(1048579))
}

func testSetQuota(t *testing.T, targetPath string) {
	mockSetQuota(t, targetPath)
	// write an file, which is smaller than the quota limit
	smallerThanQuotaFile := filepath.Join(targetPath, "smaller-than-quota")
	assert.NilError(t, ioutil.WriteFile(smallerThanQuotaFile, make([]byte, testQuotaSize/2), 0644))
	assert.NilError(t, os.Remove(smallerThanQuotaFile))

	// write an file, which is bigger than the quota limit
	biggerThanQuotaFile := filepath.Join(targetPath, "bigger-than-quota")
	err := ioutil.WriteFile(biggerThanQuotaFile, make([]byte, testQuotaSize+1), 0644)
	assert.Assert(t, is.ErrorContains(err, "no space left on device"))
	if err == io.ErrShortWrite {
		assert.NilError(t, os.Remove(biggerThanQuotaFile))
	}
}

func testGetQuota(t *testing.T, targetPath string) {
	p := mockSetQuota(t, targetPath)
	// write an file, which is smaller than the quota limit
	smallerThanQuotaFile := filepath.Join(targetPath, "smaller-than-quota")
	assert.NilError(t, ioutil.WriteFile(smallerThanQuotaFile, make([]byte, testQuotaSize/2), 0644))
	assert.NilError(t, os.Remove(smallerThanQuotaFile))

	newSize, err := p.GetQuota(targetPath)
	assert.NilError(t, err)
	assert.Equal(t, uint64(testQuotaSize), newSize.Quota)
	assert.Equal(t, newSize.InodesUsed, uint64(1))
}

func testClearQuota(t *testing.T, targetPath string) {
	p := mockSetQuota(t, targetPath)
	err := p.ClearQuota(targetPath)
	assert.NilError(t, err)

	newSize, err := p.GetQuota(targetPath)
	assert.NilError(t, err)
	assert.Equal(t, uint64(0), newSize.Quota)
}

func mockSetQuota(t *testing.T, targetPath string) *projectQuota {
	p := mockProjectQuota([]quotaID{quotaID(1048577), quotaID(1048578)})
	size := &types.DiskQuotaSize{
		Quota: testQuotaSize,
	}
	err := p.SetQuota(targetPath, types.VolumeTypeRootFs, size, nil)
	assert.NilError(t, err)
	return p
}

func mockProjectQuota(ids []quotaID) *projectQuota {
	p := &projectQuota{
		pathMapBackingDev: make(map[string]*backingDev),
		idNames:           make(map[quotaID]string),
		idPaths:           map[quotaID][]string{},
		pathIds:           make(map[string]quotaID),
		nameIds:           make(map[string]quotaID),
		prjFile:           nil,
	}
	if len(ids) != 0 {
		for i, id := range ids {
			p.idPaths[id] = []string{"/" + strings.Repeat("x", i+2)}
		}
	}

	return p
}
