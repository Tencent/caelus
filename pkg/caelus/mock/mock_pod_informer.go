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

package mock

import (
	"k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"
	"time"
)

// NewMockPodInformer news a fake pod lister for testing
// Users should assign pod info to the mock server
func NewMockPodInformer(pods []*v1.Pod) cache.SharedIndexInformer {
	source := fcache.NewFakeControllerSource()
	for i := range pods {
		p := pods[i]
		source.Add(p)
	}
	sii := cache.NewSharedInformer(source, &v1.Pod{}, 1*time.Second).(cache.SharedIndexInformer)
	return sii
}
