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

package offline

import (
	"github.com/tencent/lighthouse-plugin/pkg/plugin"
)

const (
	AnnotationKey          = plugin.PodAnnotationPrefix + "app-class"
	AnnotationOfflineValue = "greedy"
)

// IsOffline return if a pod is offline pod
// if offline is nodemanager, should be added the annotation key with greedy
func IsOffline(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	if value, ok := labels[AnnotationKey]; ok {
		if value == AnnotationOfflineValue {
			return true
		}
	}
	return false
}
