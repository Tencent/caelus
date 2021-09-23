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

package cgroup

import (
	"path"
)

const (
	PerfEventSubsystem = "perf_event"
)

// GetPerfEventCgroupPath return perf_event cgroup path
func GetPerfEventCgroupPath(pathInCgroup string) (string, error) {
	root := GetRoot()
	return path.Join(root, PerfEventSubsystem, pathInCgroup), nil
}
