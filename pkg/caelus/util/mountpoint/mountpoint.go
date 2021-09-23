/*
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
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
 * This file is copied from github.com/google/fscrypt/filesystem/,
 * and did som modification:
 * - adapt for container namespace
 * - just keep the mount point parse code to get the device mount point path
 */

package mountpoint

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"golang.org/x/sys/unix"
	"k8s.io/klog"
)

var (
	MountsInitialized = false
	mountsByDevice    map[DeviceNumber]*Mount
)

type DeviceNumber uint64

// String return format device number
func (num DeviceNumber) String() string {
	return fmt.Sprintf("%d:%d", unix.Major(uint64(num)), unix.Minor(uint64(num)))
}

// Mount describe mount info
type Mount struct {
	// mount point path
	Path string
	// xfs/ext4
	FilesystemType string
	// /dev/sda
	Device       string
	DeviceNumber DeviceNumber
	Subtree      string
	ReadOnly     bool
	Options      string
}

type mountpointTreeNode struct {
	mount    *Mount
	parent   *mountpointTreeNode
	children []*mountpointTreeNode
}

// FindMount find mount point info for the path
func FindMount(path string) (*Mount, error) {
	if err := loadMountInfo(); err != nil {
		return nil, err
	}
	deviceNumber, err := getNumberOfContainingDevice(path)
	if err != nil {
		return nil, err
	}
	mnt, ok := mountsByDevice[deviceNumber]
	if !ok {
		return nil, fmt.Errorf("couldn't find mountpoint containing %q", path)
	}
	if mnt == nil {
		return nil, fmt.Errorf("nil mount point for device %s", deviceNumber)
	}
	return mnt, nil
}

// loadMountInfo store all mount points
func loadMountInfo() error {
	if !MountsInitialized {
		file, err := os.Open("/proc/self/mountinfo")
		if err != nil {
			return err
		}
		defer file.Close()
		if err := readMountInfo(file); err != nil {
			return err
		}
		MountsInitialized = true
	}
	return nil
}

// readMountInfo parse mount point
func readMountInfo(r io.Reader) error {
	mountsByPath := make(map[string]*Mount)
	mountsByDevice = make(map[DeviceNumber]*Mount)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		mnt := parseMountInfoLine(line)
		if mnt == nil {
			klog.V(3).Infof("ignoring invalid mountinfo line %q", line)
			continue
		}

		// We can only use mountpoints that are directories for fscrypt.
		if !isDir(mnt.Path) {
			klog.V(3).Infof("ignoring mountpoint %q because it is not a directory", mnt.Path)
			continue
		}

		// Note this overrides the info if we have seen the mountpoint
		// earlier in the file. This is correct behavior because the
		// mountpoints are listed in mount order.
		mountsByPath[mnt.Path] = mnt
	}
	// For each filesystem, choose a "main" Mount and discard any additional
	// bind mounts.  fscrypt only cares about the main Mount, since it's
	// where the fscrypt metadata is stored.  Store all main Mounts in
	// mountsByDevice so that they can be found by device number later.
	allMountsByDevice := make(map[DeviceNumber][]*Mount)
	for _, mnt := range mountsByPath {
		allMountsByDevice[mnt.DeviceNumber] =
			append(allMountsByDevice[mnt.DeviceNumber], mnt)
	}
	for deviceNumber, filesystemMounts := range allMountsByDevice {
		mountsByDevice[deviceNumber] = findMainMount(filesystemMounts)
		klog.V(2).Infof("generating mount info for dev(%v): %+v",
			deviceNumber, mountsByDevice[deviceNumber])
	}
	return nil
}

// findMainMount find main mount point if the device has multi mount points
func findMainMount(filesystemMounts []*Mount) *Mount {
	// Index this filesystem's mounts by path.  Note: paths are unique here,
	// since non-last mounts were already excluded earlier.
	//
	// Also build the set of all mounted subtrees.
	mountsByPath := make(map[string]*mountpointTreeNode)
	allSubtrees := make(map[string]bool)
	for _, mnt := range filesystemMounts {
		mountsByPath[mnt.Path] = &mountpointTreeNode{mount: mnt}
		allSubtrees[mnt.Subtree] = true
	}

	// Divide the mounts into non-overlapping trees of mountpoints.
	for path, mntNode := range mountsByPath {
		for path != "/" && mntNode.parent == nil {
			path = filepath.Dir(path)
			if parent := mountsByPath[path]; parent != nil {
				mntNode.parent = parent
				parent.children = append(parent.children, mntNode)
			}
		}
	}

	// Build the set of mounted subtrees that aren't contained in any other
	// mounted subtree.
	allUncontainedSubtrees := make(map[string]bool)
	for subtree := range allSubtrees {
		contained := false
		for t := subtree; t != "/" && !contained; {
			t = filepath.Dir(t)
			contained = allSubtrees[t]
		}
		if !contained {
			allUncontainedSubtrees[subtree] = true
		}
	}

	// Select the root of a mountpoint tree whose mounted subtrees contain
	// *all* mounted subtrees.  Equivalently, select a mountpoint tree in
	// which every uncontained subtree is mounted.
	var mainMount *Mount
	for _, mntNode := range mountsByPath {
		mnt := mntNode.mount
		if mntNode.parent != nil {
			continue
		}
		uncontainedSubtrees := make(map[string]bool)
		addUncontainedSubtreesRecursive(uncontainedSubtrees, mntNode, allUncontainedSubtrees)
		if len(uncontainedSubtrees) != len(allUncontainedSubtrees) {
			continue
		}
		// If there's more than one eligible mount, they should have the
		// same Subtree.  Otherwise it's ambiguous which one to use.
		if mainMount != nil && mainMount.Subtree != mnt.Subtree {
			klog.Infof("Unsupported case: %q (%v) has multiple non-overlapping mounts. This filesystem will be ignored!",
				mnt.Device, mnt.DeviceNumber)
			// different from source code in github
			if util.InHostNamespace {
				return nil
			}
			if strings.HasPrefix(mnt.Path, types.RootFS) {
				klog.Warningf("replacing main mount(%+v) with %+v", mainMount, mnt)
				mainMount = mnt
			}
			continue
		}
		// Prefer a read-write mount to a read-only one.
		if mainMount == nil || mainMount.ReadOnly {
			mainMount = mnt
		}
	}
	return mainMount
}

func parseMountInfoLine(line string) *Mount {
	fields := strings.Split(line, " ")
	if len(fields) < 10 {
		return nil
	}

	// Count the optional fields.  In case new fields are appended later,
	// don't simply assume that n == len(fields) - 4.
	n := 6
	for fields[n] != "-" {
		n++
		if n >= len(fields) {
			return nil
		}
	}
	if n+3 >= len(fields) {
		return nil
	}

	var mnt *Mount = &Mount{}
	var err error
	mnt.DeviceNumber, err = newDeviceNumberFromString(fields[2])
	if err != nil {
		return nil
	}
	mnt.Subtree = unescapeString(fields[3])
	mnt.Path = unescapeString(fields[4])
	for _, opt := range strings.Split(fields[5], ",") {
		if opt == "ro" {
			mnt.ReadOnly = true
		}
	}
	mnt.FilesystemType = unescapeString(fields[n+1])
	mnt.Device = getDeviceName(mnt.DeviceNumber)
	mnt.Options = fields[len(fields)-1]
	return mnt
}

func addUncontainedSubtreesRecursive(dst map[string]bool,
	node *mountpointTreeNode, allUncontainedSubtrees map[string]bool) {
	if allUncontainedSubtrees[node.mount.Subtree] {
		dst[node.mount.Subtree] = true
	}
	for _, child := range node.children {
		addUncontainedSubtreesRecursive(dst, child, allUncontainedSubtrees)
	}
}

func loggedStat(name string) (os.FileInfo, error) {
	info, err := os.Stat(name)
	if err != nil && !os.IsNotExist(err) {
		klog.Error(err)
	}
	return info, err
}

// isDir returns true if the path exists and is that of a directory.
func isDir(path string) bool {
	info, err := loggedStat(path)
	return err == nil && info.IsDir()
}

func newDeviceNumberFromString(str string) (DeviceNumber, error) {
	var major, minor uint32
	if count, _ := fmt.Sscanf(str, "%d:%d", &major, &minor); count != 2 {
		return 0, fmt.Errorf("invalid device number string %q", str)
	}
	return DeviceNumber(unix.Mkdev(major, minor)), nil
}

func unescapeString(str string) string {
	var sb strings.Builder
	for i := 0; i < len(str); i++ {
		b := str[i]
		if b == '\\' && i+3 < len(str) {
			if parsed, err := strconv.ParseInt(str[i+1:i+4], 8, 8); err == nil {
				b = uint8(parsed)
				i += 3
			}
		}
		sb.WriteByte(b)
	}
	return sb.String()
}

func getDeviceName(num DeviceNumber) string {
	linkPath := fmt.Sprintf("/sys/dev/block/%v", num)
	if target, err := os.Readlink(linkPath); err == nil {
		devDir := "/dev"
		if !util.InHostNamespace {
			devDir = path.Join(types.RootFS, devDir)
		}
		return path.Join(devDir, filepath.Base(target))
	}
	return ""
}

func getNumberOfContainingDevice(path string) (DeviceNumber, error) {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return 0, err
	}
	return DeviceNumber(stat.Dev), nil
}
