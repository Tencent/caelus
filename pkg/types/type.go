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

package types

import "syscall"

// DiskPartitionStats show disk space size
type DiskPartitionStats struct {
	// TotalSize show total disk size in bytes
	TotalSize int64
	// UsedSize show used disk size in bytes
	UsedSize int64
	// FreeSize show free disk size in bytes
	FreeSize int64
}

// GetDiskPartitionStats output disk space stats for the partition
func GetDiskPartitionStats(partitionName string) (*DiskPartitionStats, error) {
	stat := syscall.Statfs_t{}

	err := syscall.Statfs(partitionName, &stat)
	if err != nil {
		return nil, err
	}

	dStats := &DiskPartitionStats{
		TotalSize: int64(stat.Blocks) * stat.Bsize,
		FreeSize:  int64(stat.Bfree) * stat.Bsize,
	}
	dStats.UsedSize = dStats.TotalSize - dStats.FreeSize
	return dStats, nil
}
