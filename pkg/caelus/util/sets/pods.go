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

package sets

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type Pod map[string]*v1.Pod

// NewPod create Pod instance
func NewPod(items ...*v1.Pod) Pod {
	ps := Pod{}
	ps.Insert(items...)
	return ps
}

// Insert add new pods
func (p Pod) Insert(items ...*v1.Pod) Pod {
	for _, item := range items {
		key, err := cache.MetaNamespaceKeyFunc(item)
		if err != nil {
			klog.Warningf("can't get key for pod %+#v, %v", item, err)
			continue
		}
		p[key] = item
	}
	return p
}

// Delete delete old pods
func (p Pod) Delete(items ...*v1.Pod) Pod {
	for _, item := range items {
		key, err := cache.MetaNamespaceKeyFunc(item)
		if err != nil {
			klog.Warningf("can't get key for pod %+#v, %v", item, err)
			continue
		}
		delete(p, key)
	}
	return p
}

// Has function check if the pod existed
func (p Pod) Has(item *v1.Pod) bool {
	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		klog.Warningf("can't get key for pod %+#v, %v", item, err)
		return false
	}

	_, found := p[key]
	return found
}

// UnsortedList return pod list
func (p Pod) UnsortedList() []*v1.Pod {
	l := make([]*v1.Pod, 0, len(p))
	for _, v := range p {
		l = append(l, v)
	}
	return l
}

// Update update pod
func (p Pod) Update(item *v1.Pod) bool {
	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		klog.Warningf("can't get key for pod %+#v, %v", item, err)
		return false
	}

	p[key] = item
	return true
}
