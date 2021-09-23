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
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
)

const QosDiskIO = "diskio"

// qosDiskIO manage offline job disk io resource
type qosDiskIO struct {
	disks []string
}

// NewQosDiskIO creates disk io manager
func NewQosDiskIO(disks []string) ResourceQosManager {
	return &qosDiskIO{
		disks: disks,
	}
}

// Name returns resource policy name
func (d *qosDiskIO) Name() string {
	return QosDiskIO
}

// PreInit do nothing
func (d *qosDiskIO) PreInit() error {
	return nil
}

// Run starts nothing
func (d *qosDiskIO) Run(stop <-chan struct{}) {}

// ManageDiskIO isolates disk io resource for offline jobs
func (d *qosDiskIO) Manage(cgResources *CgroupResourceConfig) error {
	offlineParent := types.CgroupOffline
	return cgroup.SetBlkioWeight(offlineParent, d.disks)
}
