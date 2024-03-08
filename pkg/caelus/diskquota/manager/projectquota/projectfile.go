/*
 * Copyright The Kubernetes Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 *
 * For file operations, we learn something from
 * https://github.com/kubernetes/kubernetes/pkg/volume/util/fsquota/project.go
 */

package projectquota

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"k8s.io/klog/v2"
)

var (
	tmpPrefix = "."

	projectsPath = "/etc/projects"
	projidPath   = "/etc/projid"
)

// projectFile used to record project quota to file
type projectFile struct{}

// NewProjectFile new project file instance
func NewProjectFile() *projectFile {
	if !util.InHostNamespace {
		projectsPath = path.Join(types.RootFS, projectsPath)
		projidPath = path.Join(types.RootFS, projidPath)
	}
	if err := projFilesAreOK(); err != nil {
		klog.Fatalf("project files are not ok: %v", err)
	}

	return &projectFile{}
}

// DumpProjectIds read project quota record
func (f *projectFile) DumpProjectIds() (idPaths map[quotaID][]string, idNames map[quotaID]string, err error) {
	idPaths = make(map[quotaID][]string)
	idNames = make(map[quotaID]string)

	// 1048579:/data1/test
	idPathHandleFunc := func(ranges []string) {
		idStr := strings.TrimSpace(ranges[0])
		id, err := strconv.Atoi(idStr)
		if err != nil {
			klog.Errorf("invalid project id(%s): %v", idStr, err)
			return
		}

		paths := []string{}
		pathName := strings.TrimSpace(ranges[1])
		if !util.InHostNamespace {
			pathName = path.Join(types.RootFS, pathName)

		}

		if loadedPaths, exist := idPaths[quotaID(id)]; exist == true {
			paths = append(loadedPaths, pathName)
		} else {
			paths = append(paths, pathName)
		}

		idPaths[quotaID(id)] = paths
	}
	// emptyDir-1048577:1048577
	idNameHandleFunc := func(ranges []string) {
		idName := strings.TrimSpace(ranges[0])
		idStr := strings.TrimSpace(ranges[1])
		id, err := strconv.Atoi(idStr)
		if err != nil {
			klog.Errorf("invalid project id(%s): %v", idStr, err)
			return
		}
		idNames[quotaID(id)] = idName
	}
	dumpProjectsFile(projectsPath, idPathHandleFunc)
	dumpProjectsFile(projidPath, idNameHandleFunc)

	klog.V(2).Infof("dump new project paths: %+v", idPaths)
	klog.V(2).Infof("dump new project ids: %+v", idNames)
	return idPaths, idNames, nil
}

// UpdateProjects save projectid:path to /etc/projects
func (f *projectFile) UpdateProjects(idPaths map[quotaID][]string) error {
	klog.V(2).Infof("update new project ids to file: %v", idPaths)

	projectsStr := ""
	for id, paths := range idPaths {
		for _, path := range paths {
			if !util.InHostNamespace {
				path = strings.TrimPrefix(path, types.RootFS)
			}
			projectsStr += fmt.Sprintf("%d:%s\n", id, path)
		}
	}

	klog.V(4).Infof("written projects: %s", projectsStr)
	err := writeByTempFile(projectsPath, []byte(projectsStr))
	if err != nil {
		klog.Errorf("write projects file(%s) failed: %v", projectsPath, err)
		klog.Errorf("the written text: %s", projectsStr)
	}
	return err
}

// UpdateProjIds save projectName:id to /etc/projid
func (f *projectFile) UpdateProjIds(idNames map[quotaID]string) error {
	klog.V(2).Infof("update new project ids to file: %v", idNames)

	projidStr := ""
	for id, name := range idNames {
		projidStr += fmt.Sprintf("%s:%d\n", name, id)
	}

	klog.V(4).Infof("written projids: %s", projidStr)
	err := writeByTempFile(projidPath, []byte(projidStr))

	if err != nil {
		klog.Errorf("write project ids file(%s) failed: %v", projidPath, err)
		klog.Errorf("the written text: %s", projidStr)
	}

	return err
}

func projFilesAreOK() error {
	klog.V(2).Infof("projects: %s, projid: %s", projectsPath, projidPath)

	if sf, err := os.Lstat(projectsPath); err != nil || sf.Mode().IsRegular() {
		if sf, err := os.Lstat(projidPath); err != nil || sf.Mode().IsRegular() {
			return nil
		}
		return fmt.Errorf("%s exists but is not a plain file, cannot continue", projidPath)
	}
	return fmt.Errorf("%s exists but is not a plain file, cannot continue", projectsPath)
}

func dumpProjectsFile(filePath string, handle func(ranges []string)) {
	prj, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		klog.Errorf("open project file(%s) err: %v", filePath, err)
		return
	}
	defer prj.Close()

	scanner := bufio.NewScanner(prj)
	for scanner.Scan() {
		ranges := strings.Split(scanner.Text(), ":")
		if len(ranges) != 2 {
			continue
		}
		handle(ranges)
	}
}

func writeByTempFile(pathFile string, data []byte) (retErr error) {
	// Create a temporary file in the base directory of `path` with a prefix.
	tmpFile, err := ioutil.TempFile(filepath.Dir(pathFile), tmpPrefix)
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	shouldClose := true

	defer func() {
		// Close the file.
		if shouldClose {
			if err := tmpFile.Close(); err != nil {
				klog.Errorf("close tmp file(%s) err: %v", tmpPath, err)
			}
		}

		// Clean up the temp file on error.
		if retErr != nil && tmpPath != "" {
			if err := os.Remove(tmpPath); err != nil {
				if !os.IsNotExist(err) {
					klog.Errorf("remove tmp file(%s) error: %v", tmpPath, err)
				}
			}
		}
	}()

	// Write data.
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}

	// Sync file.
	if err := tmpFile.Sync(); err != nil {
		return err
	}

	// Closing the file before renaming.
	err = tmpFile.Close()
	shouldClose = false
	if err != nil {
		return err
	}

	return os.Rename(tmpPath, pathFile)
}
