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
	"errors"

	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/apimachinery/pkg/util/sets"
)

var NotSupported = errors.New("not suppported")

// QuotaManager describes functions for quota manager
type QuotaManager interface {
	// GetQuota get disk quota for the target path
	GetQuota(targetPath string) (*types.DiskQuotaSize, error)
	// SetQuota set disk quota for path. pathFlag shows which the path is, such as emptyDir or hostPath
	// if sharedInfo is not nil, quota size is for a group of path of this pod
	SetQuota(targetPath string, pathFlag types.VolumeType, size *types.DiskQuotaSize, sharedInfo *types.SharedInfo) error
	// ClearQuota clears quota
	ClearQuota(targetPath string) error
	// GetAllQuotaPath return all paths which has set quota, and classified by path flag
	GetAllQuotaPath() map[types.VolumeType]sets.String
}

// IsNotSupported check if is not supported error
func IsNotSupported(err error) bool {
	return err == NotSupported
}
