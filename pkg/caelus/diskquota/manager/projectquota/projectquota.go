/*
 * Copyright The moby Authors
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
 * This file is copied from https://github.com/moby/moby/blob/master/quota/projectquota.go
 * and chose the functions how to set/get project quota via system call
 */

package projectquota

/*
#include <stdlib.h>
#include <dirent.h>
#include <linux/fs.h>
#include <linux/quota.h>
#include <linux/dqblk_xfs.h>

#ifndef FS_XFLAG_PROJINHERIT
struct fsxattr {
	__u32		fsx_xflags;
	__u32		fsx_extsize;
	__u32		fsx_nextents;
	__u32		fsx_projid;
	unsigned char	fsx_pad[12];
};
#define FS_XFLAG_PROJINHERIT	0x00000200
#endif
#ifndef FS_IOC_FSGETXATTR
#define FS_IOC_FSGETXATTR		_IOR ('X', 31, struct fsxattr)
#endif
#ifndef FS_IOC_FSSETXATTR
#define FS_IOC_FSSETXATTR		_IOW ('X', 32, struct fsxattr)
#endif

#ifndef PRJQUOTA
#define PRJQUOTA	2
#endif
#ifndef XFS_PROJ_QUOTA
#define XFS_PROJ_QUOTA	2
#endif
#ifndef Q_XSETPQLIM
#define Q_XSETPQLIM QCMD(Q_XSETQLIM, PRJQUOTA)
#endif
#ifndef Q_XGETPQUOTA
#define Q_XGETPQUOTA QCMD(Q_XGETQUOTA, PRJQUOTA)
#endif

const int Q_XGETQSTAT_PRJQUOTA = QCMD(Q_XGETQSTAT, PRJQUOTA);
*/
import "C"
import (
	"fmt"
	"path"
	"strings"
	"unsafe"

	"github.com/tencent/caelus/pkg/caelus/diskquota/manager"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/mountpoint"

	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	noQuotaID    quotaID = 0
	firstQuotaID quotaID = 1048577
	// Don't go into an infinite loop searching for an unused quota id
	maxSearch        = 256
	idNameSeprator   = "-"
	quotaMountOption = "prjquota"
)
const (
	projIdNoCreate = true
	persistToFile  = true
)

// quotaID is generic quota identifier.
// Data type based on quotactl(2).
type quotaID int32

// String returns quota id in string format
func (q quotaID) String() string {
	return fmt.Sprintf("%d", q)
}

// IdName returns quota id with path flag
func (q quotaID) IdName(pathFlag string) string {
	return fmt.Sprintf("%s%s%d", pathFlag, idNameSeprator, q)
}

type projectQuota struct {
	// path => device
	pathMapBackingDev map[string]*backingDev
	// id => id with path flag
	idNames map[quotaID]string
	// id => path
	idPaths map[quotaID][]string
	// path => id
	pathIds map[string]quotaID
	// store project quota to file
	nameIds map[string]quotaID
	prjFile *projectFile
}

// ioctl using the backingDev to set disk quota
type backingDev struct {
	supported bool
	device    string
}

// NewProjectQuota creates a new project quota manager
func NewProjectQuota() manager.QuotaManager {
	prjFile := NewProjectFile()
	idPaths, idNames, err := prjFile.DumpProjectIds()
	if err != nil {
		klog.Fatalf("init existing project ids failed: %v", err)
	}
	pathIds := make(map[string]quotaID)
	for id, paths := range idPaths {
		for _, p := range paths {
			pathIds[p] = id
		}
	}

	nameIds := make(map[string]quotaID)
	for id, name := range idNames {
		nameIds[name] = id

	}

	return &projectQuota{
		pathMapBackingDev: make(map[string]*backingDev),
		idPaths:           idPaths,
		idNames:           idNames,
		pathIds:           pathIds,
		nameIds:           nameIds,
		prjFile:           prjFile,
	}
}

// GetQuota get disk quota for the target path
func (p *projectQuota) GetQuota(targetPath string) (*types.DiskQuotaSize, error) {
	backingFsBlockDev, err := p.findAvailableBackingDev(targetPath)
	if err != nil {
		return nil, err
	}

	// no need to create new project id
	prjId, _, err := p.findOrCreateProjectId(targetPath, "", projIdNoCreate, !persistToFile)
	if err != nil {
		klog.Errorf("find project id err: %v", err)
		return nil, err
	}

	return getProjectQuota(backingFsBlockDev.device, prjId)
}

// SetQuota set quota limit for the path
func (p *projectQuota) SetQuota(targetPath string, pathFlag types.VolumeType, size *types.DiskQuotaSize,
	sharedInfo *types.SharedInfo) error {
	if size == nil || (size.Quota+size.Inodes == 0) {
		return nil
	}

	backingFsBlockDev, err := p.findAvailableBackingDev(targetPath)
	if err != nil {
		return err
	}

	var prjId quotaID
	var isNewId bool
	if sharedInfo != nil {
		prjId, isNewId, err = p.findOrCreateSharedProjectId(targetPath, pathFlag.String(), sharedInfo.PodName)
		if err != nil {
			return fmt.Errorf("find or create project id err: %v", err)
		}
	} else {

		prjId, isNewId, err = p.findOrCreateProjectId(targetPath, pathFlag.String(), !projIdNoCreate, persistToFile)
		if err != nil {
			return fmt.Errorf("find or create project id err: %v", err)
		}
	}

	if isNewId {
		klog.V(2).Infof("set disk quota for path(%s) with size: %+v", targetPath, size)
		err = setProjectQuota(backingFsBlockDev.device, prjId, size)
		if err != nil {
			klog.Errorf("set disk quota for path(%s) with size(%+v) failed: %v", targetPath, size, err)
		}

	} else {
		klog.V(4).Infof("disk quota for path(%s) has already set", targetPath)
	}

	return err
}

// ClearQuota clear quota limit for the path
func (p *projectQuota) ClearQuota(targetPath string) error {
	backingFsBlockDev, err := p.findAvailableBackingDev(targetPath)
	if err != nil {
		return err
	}

	// no need to create new project id
	prjId, _, err := p.findOrCreateProjectId(targetPath, "", projIdNoCreate, persistToFile)
	if err != nil {
		klog.Errorf("find project id err: %v", err)
		return err
	}

	size := &types.DiskQuotaSize{
		Quota:  0,
		Inodes: 0,
	}
	err = setProjectQuota(backingFsBlockDev.device, prjId, size)
	if err != nil {
		// just warning
		klog.Errorf("set zero quota failed for path(%s) with id(%d): %v", targetPath, prjId, err)
	}

	// save
	projName, ok := p.idNames[prjId]
	delete(p.pathIds, targetPath)
	delete(p.pathMapBackingDev, targetPath)
	delete(p.idPaths, prjId)
	delete(p.idNames, prjId)
	if ok {
		delete(p.nameIds, projName)
	}

	// save to file
	if p.prjFile != nil {
		p.prjFile.UpdateProjects(p.idPaths)
		p.prjFile.UpdateProjIds(p.idNames)
	}

	return nil
}

// GetAllQuotaPath returns existing quota paths
func (p *projectQuota) GetAllQuotaPath() map[types.VolumeType]sets.String {
	paths := make(map[types.VolumeType]sets.String)

	for id, pathGroup := range p.idPaths {
		for _, path := range pathGroup {
			idName := p.idNames[id]
			ranges := strings.Split(idName, idNameSeprator)
			pathFlag := types.VolumeType(ranges[0])
			if _, ok := paths[pathFlag]; ok {
				paths[pathFlag].Insert(path)
			} else {
				paths[pathFlag] = sets.NewString(path)
			}
		}
	}

	return paths
}

// findAvailableBackingDev checks NotSupported error
func (p *projectQuota) findAvailableBackingDev(targetPath string) (*backingDev, error) {
	backingFsBlockDev, err := p.findOrCreateBackingDev(targetPath)
	if err != nil {
		return nil, err
	}
	if !backingFsBlockDev.supported {
		return nil, manager.NotSupported
	}

	return backingFsBlockDev, nil
}

// findOrCreateBackingDev will create a fake device to set project quota,
// the fake device will be sharing among the paths with the same mount point,
// so the function will find the mount point and create a sharing fake device.
func (p *projectQuota) findOrCreateBackingDev(targetPath string) (*backingDev, error) {
	if dev, ok := p.pathMapBackingDev[targetPath]; ok {
		return dev, nil
	}

	var stat unix.Stat_t
	if err := unix.Stat(targetPath, &stat); err != nil {
		return nil, err
	}

	// if the target path is soft link, the function can also find the right point
	mountInfo, err := mountpoint.FindMount(targetPath)
	if err != nil {
		return nil, err
	}
	mountPoint := mountInfo.Path
	klog.V(3).Infof("mount point for path(%s) is: %s", targetPath, mountPoint)
	if dev, ok := p.pathMapBackingDev[mountPoint]; ok {
		p.pathMapBackingDev[targetPath] = dev
		return dev, nil
	}

	// create block device
	backingFsBlockDev := path.Join(mountPoint, "backingFsBlockDev")
	backingDevice := &backingDev{
		device: backingFsBlockDev,
	}
	p.pathMapBackingDev[mountPoint] = backingDevice
	p.pathMapBackingDev[targetPath] = backingDevice

	// if mount options has no quota option, no quota support
	if !strings.Contains(mountInfo.Options, quotaMountOption) {
		backingDevice.supported = false
		klog.V(2).Infof("generating backing device for path(%s) with mount point(%s): %+v",
			targetPath, mountPoint, backingDevice)
		return backingDevice, nil
	}

	// check if the mount point supporting disk quota
	_, err = getProjectID(mountPoint)
	if err != nil && strings.Contains(err.Error(), "inappropriate ioctl for device") {
		backingDevice.supported = false
	} else {
		klog.V(2).Infof("creating backing device for target path(%s): %s",
			targetPath, backingFsBlockDev)
		// Re-create just in case someone copied the home directory over to a new device
		unix.Unlink(backingFsBlockDev)
		err = unix.Mknod(backingFsBlockDev, unix.S_IFBLK|0600, int(stat.Dev))
		switch err {
		case nil:
			backingDevice.supported = true
		case unix.ENOSYS, unix.EPERM:
			backingDevice.supported = false
		default:
			return nil, fmt.Errorf("failed to mknod %s: %v", backingFsBlockDev, err)
		}
	}

	klog.V(2).Infof("generating backing device for path(%s) with mount point(%s): %+v",
		targetPath, mountPoint, backingDevice)
	return backingDevice, nil
}

//findOrCreateSharedProjectId check if the path already has an shared project id, creating if not.
func (p *projectQuota) findOrCreateSharedProjectId(targetPath, pathFlag string,
	groupName string) (quotaID, bool, error) {

	isNewId := false
	if groupName == "" {
		return 0, isNewId, fmt.Errorf("wrong group name")
	}

	// find an shared project id and bind it to path
	// format: emptyDir-namespace/podname:1048578
	projName := fmt.Sprintf("%s-%s", pathFlag, groupName)
	if prjId, ok := p.nameIds[projName]; ok == true {
		update, err := p.bindProjectId(targetPath, prjId)
		if err == nil && update && p.prjFile != nil {
			// update /etc/projects
			p.prjFile.UpdateProjects(p.idPaths)
		}
		return prjId, isNewId, err
	}

	// shared projid not existing and create one.
	isNewId = true
	projId, _, err := p.findOrCreateProjectId(targetPath, groupName, !projIdNoCreate, !persistToFile)
	if err != nil {
		return 0, false, err
	}
	p.nameIds[projName] = projId
	// modify idNames set in findOrCreateProjectId
	p.idNames[projId] = projName
	if p.prjFile != nil {
		p.prjFile.UpdateProjects(p.idPaths)
		p.prjFile.UpdateProjIds(p.idNames)
	}
	return projId, true, nil
}

// bindProjectId bind the project id to path
func (p *projectQuota) bindProjectId(targetPath string, projectId quotaID) (bool, error) {

	var paths []string
	updated := false

	saveId, ok := p.pathIds[targetPath]
	if ok == false || saveId != projectId {
		klog.V(2).Infof("binding project id(%d) to path %s", projectId, targetPath)
		err := setProjectID(targetPath, projectId)
		if err != nil {
			return updated, fmt.Errorf("set project id failed: %v", err)
		}
		p.pathIds[targetPath] = projectId
		if paths, ok = p.idPaths[projectId]; ok == false {
			paths = []string{targetPath}
			klog.Errorf("This should not be the first path(%s) for shared project id(%d)", targetPath, projectId)

		} else {
			paths = append(paths, targetPath)
		}
		p.idPaths[projectId] = paths
		updated = true
	} else {
		klog.V(2).Infof("project id(%d) has binded to path(%s), do nothing", projectId, targetPath)
	}

	return updated, nil
}

// findOrCreateProjectId check if the path already has an project id, creating if not.
func (p *projectQuota) findOrCreateProjectId(targetPath, pathFlag string,
	noCreate bool, persist bool) (quotaID, bool, error) {
	isNewId := false

	prjid, ok := p.pathIds[targetPath]
	if ok {
		return prjid, isNewId, nil
	}

	isNewId = true
	idFromSys, err := getProjectID(targetPath)
	if err != nil {
		klog.Errorf("get project id for path %s err: %v", targetPath, err)
		return 0, isNewId, err
	}
	klog.V(2).Infof("project id from sys for path(%s): %d", targetPath, idFromSys)
	if noCreate {
		return idFromSys, isNewId, nil
	}

	// check if the project id is available, generating new id if not available.
	needNewId := false
	if idFromSys == noQuotaID {
		// project id has no quota, need to allocate another project id
		needNewId = true
	} else {
		// project id is not zero, checking if the project id is in using
		if path, ok := p.idPaths[idFromSys]; ok {
			// the id has already been used, but not the target path, need generating new id
			klog.V(2).Infof("projcect id(%d) from sys for %s is already in use by paths(%v)",
				idFromSys, targetPath, path)
			needNewId = true
		} else {
			// not found from existing project id sets, using directly
			needNewId = false
		}
	}

	newId := idFromSys
	if needNewId {
		newId = p.allocateProjectID(targetPath)
		if newId == noQuotaID {
			return noQuotaID, isNewId, fmt.Errorf("allocate new project id failed")
		}

		klog.V(2).Infof("allocate new project id(%d), binding to path %s", newId, targetPath)
		err = setProjectID(targetPath, newId)
		if err != nil {
			return noQuotaID, isNewId, fmt.Errorf("set project id failed: %v", err)
		}
	}

	// save to file
	klog.V(2).Infof("add new project id: %d-%s", newId, targetPath)
	p.pathIds[targetPath] = newId
	p.idPaths[newId] = []string{targetPath}
	p.idNames[newId] = newId.IdName(pathFlag)
	// save to file
	if p.prjFile != nil && persist {
		p.prjFile.UpdateProjects(p.idPaths)
		p.prjFile.UpdateProjIds(p.idNames)
	}

	return newId, isNewId, nil
}

// allocateProjectID allocates a new project id
func (p *projectQuota) allocateProjectID(targetPath string) quotaID {
	searched := 0
	for id := firstQuotaID; id == id; id++ {
		if _, ok := p.idPaths[id]; ok {
			continue
		}

		// check if the project id is available
		backingFsBlockDev, err := p.findOrCreateBackingDev(targetPath)
		if err != nil || !backingFsBlockDev.supported {
			klog.Errorf("disk quota backing dev check failed for %s when allocating new project id", targetPath)
			return noQuotaID
		}
		quota, err := getProjectQuota(backingFsBlockDev.device, id)
		if err != nil {
			// if the project id has not been allocated, it will output the no such file error
			if strings.Contains(err.Error(), "no such file or directory") ||
				strings.Contains(err.Error(), "no such process") {
				return id
			} else {
				klog.Errorf("get project quota for %s failed when allocating new project id: %v",
					backingFsBlockDev.device, err)
				return noQuotaID
			}
		}
		if quota.Quota == 0 {
			return id
		}

		searched++
		// in case loop infinitely
		if searched > maxSearch {
			klog.Errorf("find available project id reach to max(%d) for %s when allocating new project id",
				maxSearch, targetPath)
			break
		}
	}

	return noQuotaID
}

// getProjectQuota gets disk quota based on project id
func getProjectQuota(backingFsBlockDev string, projectID quotaID) (*types.DiskQuotaSize, error) {
	var d C.fs_disk_quota_t

	var cs = C.CString(backingFsBlockDev)
	defer C.free(unsafe.Pointer(cs))

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, C.Q_XGETPQUOTA,
		uintptr(unsafe.Pointer(cs)), uintptr(C.__u32(projectID)),
		uintptr(unsafe.Pointer(&d)), 0, 0)
	if errno != 0 {
		return nil, fmt.Errorf("failed to get quota limit for projid %d on %s: %v",
			projectID, backingFsBlockDev, errno)
	}

	return &types.DiskQuotaSize{
		Quota:      uint64(d.d_blk_hardlimit) * 512,
		Inodes:     uint64(d.d_ino_hardlimit),
		QuotaUsed:  uint64(d.d_bcount) * 512,
		InodesUsed: uint64(d.d_icount),
	}, nil
}

// setProjectQuota - set the quota for project id on xfs block device
func setProjectQuota(backingFsBlockDev string, projectID quotaID, quota *types.DiskQuotaSize) error {
	klog.V(4).Infof("Setting projec quota for %d: %+v", projectID, quota)

	var d C.fs_disk_quota_t
	d.d_version = C.FS_DQUOT_VERSION
	d.d_id = C.__u32(projectID)
	d.d_flags = C.XFS_PROJ_QUOTA

	d.d_fieldmask = C.FS_DQ_BHARD | C.FS_DQ_BSOFT | C.FS_DQ_IHARD | C.FS_DQ_ISOFT
	d.d_blk_hardlimit = C.__u64(quota.Quota / 512)
	d.d_blk_softlimit = d.d_blk_hardlimit
	d.d_ino_hardlimit = C.__u64(quota.Inodes)
	d.d_ino_softlimit = d.d_ino_hardlimit

	var cs = C.CString(backingFsBlockDev)
	defer C.free(unsafe.Pointer(cs))

	_, _, errno := unix.Syscall6(unix.SYS_QUOTACTL, C.Q_XSETPQLIM,
		uintptr(unsafe.Pointer(cs)), uintptr(d.d_id),
		uintptr(unsafe.Pointer(&d)), 0, 0)
	if errno != 0 {
		return fmt.Errorf("failed to set quota limit for projid %d on %s: %v",
			projectID, backingFsBlockDev, errno)
	}

	return nil
}

// getProjectID - get the project id of path on xfs, also work for soft link path
func getProjectID(targetPath string) (quotaID, error) {
	dir, err := openDir(targetPath)
	if err != nil {
		return 0, err
	}
	defer closeDir(dir)

	var fsx C.struct_fsxattr
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, getDirFd(dir), C.FS_IOC_FSGETXATTR,
		uintptr(unsafe.Pointer(&fsx)))
	if errno != 0 {
		return 0, fmt.Errorf("failed to get projid for %s: %v", targetPath, errno)
	}

	return quotaID(fsx.fsx_projid), nil
}

// setProjectID - set the project id of path on xfs, also work for soft link path
func setProjectID(targetPath string, projectID quotaID) error {
	dir, err := openDir(targetPath)
	if err != nil {
		return err
	}
	defer closeDir(dir)

	var fsx C.struct_fsxattr
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, getDirFd(dir), C.FS_IOC_FSGETXATTR,
		uintptr(unsafe.Pointer(&fsx)))
	if errno != 0 {
		return fmt.Errorf("failed to get projid for %s: %v", targetPath, errno)
	}
	fsx.fsx_projid = C.__u32(projectID)
	fsx.fsx_xflags |= C.FS_XFLAG_PROJINHERIT
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, getDirFd(dir), C.FS_IOC_FSSETXATTR,
		uintptr(unsafe.Pointer(&fsx)))
	if errno != 0 {
		return fmt.Errorf("failed to set projid for %s: %v", targetPath, errno)
	}

	return nil
}

func free(p *C.char) {
	C.free(unsafe.Pointer(p))
}

func openDir(path string) (*C.DIR, error) {
	Cpath := C.CString(path)
	defer free(Cpath)

	dir := C.opendir(Cpath)
	if dir == nil {
		return nil, fmt.Errorf("failed to open dir: %s", path)
	}
	return dir, nil
}

func closeDir(dir *C.DIR) {
	if dir != nil {
		C.closedir(dir)
	}
}

func getDirFd(dir *C.DIR) uintptr {
	return uintptr(C.dirfd(dir))
}
