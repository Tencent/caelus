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

package cgroupstore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/mock"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var fakePods = []*v1.Pod{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "guarnateed",
			Namespace: "default",
			Annotations: map[string]string{
				"test": "online",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "guarnateed",
			Namespace: "kube-system",
			Annotations: map[string]string{
				"test": "online",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "besteffort",
			Namespace: "default",
			Annotations: map[string]string{
				"test": "online",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "offlinePod",
			Namespace: "default",
			Annotations: map[string]string{
				appclass.AnnotationOfflineKey: appclass.AnnotationOfflineValue,
			},
		},
	},
}

type refTestData struct {
	describe string
	cgPath   string
	conStat  cadvisorapiv2.ContainerInfo
	expect   struct {
		refData *CgroupRef
		key     string
		err     error
	}
}

var generateRefTestCases = []refTestData{
	{
		describe: "normal guaranteed pod in default ns",
		cgPath: "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
			"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{
					"io.kubernetes.pod.name":       "guarnateed",
					"io.kubernetes.pod.namespace":  "default",
					"io.kubernetes.pod.uid":        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
					"io.kubernetes.container.name": "guaranteedCon",
				},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: &CgroupRef{
				Name: "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
					"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
				ContainerName: "guaranteedCon",
				PodName:       "guarnateed",
				PodNamespace:  "default",
				PodUid:        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
				AppClass:      appclass.AppClassOnline,
			},
			key: "2c85e_guaranteedCon",
			err: nil,
		},
	},
	{
		describe: "normal guaranteed pod in kube-system ns",
		cgPath: "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
			"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{
					"io.kubernetes.pod.name":       "guarnateed",
					"io.kubernetes.pod.namespace":  "kube-system",
					"io.kubernetes.pod.uid":        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
					"io.kubernetes.container.name": "guaranteedCon",
				},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: &CgroupRef{
				Name: "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
					"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
				ContainerName: "guaranteedCon",
				PodName:       "guarnateed",
				PodNamespace:  "kube-system",
				PodUid:        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
				AppClass:      appclass.AppClassSystem,
			},
			key: "2c85e_guaranteedCon",
			err: nil,
		},
	},
	{
		describe: "normal besteffort pod in defalut ns",
		cgPath: "/kubepods/besteffort/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
			"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{
					"io.kubernetes.pod.name":       "besteffort",
					"io.kubernetes.pod.namespace":  "default",
					"io.kubernetes.pod.uid":        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
					"io.kubernetes.container.name": "besteffortCon",
				},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: &CgroupRef{
				Name: "/kubepods/besteffort/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
					"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
				ContainerName: "besteffortCon",
				PodName:       "besteffort",
				PodNamespace:  "default",
				PodUid:        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
				AppClass:      appclass.AppClassOnline,
			},
			key: "2c85e_besteffortCon",
			err: nil,
		},
	},
	{
		describe: "normal offline pod",
		cgPath: "/kubepods/offline/besteffort/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
			"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{
					"io.kubernetes.pod.name":       "offlinePod",
					"io.kubernetes.pod.namespace":  "default",
					"io.kubernetes.pod.uid":        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
					"io.kubernetes.container.name": "offlineCon",
				},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: &CgroupRef{
				Name: "/kubepods/offline/besteffort/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
					"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
				ContainerName: "offlineCon",
				PodName:       "offlinePod",
				PodNamespace:  "default",
				PodUid:        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
				AppClass:      appclass.AppClassOffline,
			},
			key: "2c85e_offlineCon",
			err: nil,
		},
	},
	{
		describe: "infra pod",
		cgPath: "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
			"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{
					"io.kubernetes.pod.name":       "guarnateed",
					"io.kubernetes.pod.namespace":  "default",
					"io.kubernetes.pod.uid":        "2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
					"io.kubernetes.container.name": PodInfraContainerName,
				},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: nil,
			key:     "",
			err:     fmt.Errorf("cgroup path is infra container"),
		},
	},
	{
		describe: "non-pod online cgroup",
		cgPath:   "/onlinejobs/online",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: &CgroupRef{
				Name:          "/onlinejobs/online",
				ContainerName: "online",
				PodName:       "",
				PodNamespace:  "",
				PodUid:        "",
				AppClass:      appclass.AppClassOnline,
			},
			key: "online",
			err: nil,
		},
	},
	{
		describe: "non-pod cgroup",
		cgPath:   "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: nil,
			key:     "",
			err:     fmt.Errorf("cgroup path is podxxx"),
		},
	},
	{
		describe: "non-pod aggregatedcgroup",
		cgPath:   "/kubepods",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: &CgroupRef{
				Name:          "/kubepods",
				ContainerName: "kubepods",
				PodName:       "",
				PodNamespace:  "",
				PodUid:        "",
				AppClass:      appclass.AppClassAggregatedCgroup,
			},
			key: "kubepods",
			err: nil,
		},
	},
	{
		describe: "long cgroup path",
		cgPath: "/kubepods/pod2c85e5c5-bf6b-49f3-bb51-35d6bf02d823/" +
			"efda7affa51916b680120de580a1acd2d861c59a42e4d3ed7733a0697f389384/system.slice/crond.service/",
		conStat: cadvisorapiv2.ContainerInfo{
			Spec: cadvisorapiv2.ContainerSpec{
				Labels: map[string]string{},
			},
		},
		expect: struct {
			refData *CgroupRef
			key     string
			err     error
		}{
			refData: nil,
			key:     "",
			err:     fmt.Errorf("cgroup path too long"),
		},
	},
}

// TestGenerateRef test generates cgroup reference data
func TestGenerateRef(t *testing.T) {
	stopCh := make(chan struct{}, 1)
	podInformer := mock.NewMockPodInformer(fakePods)
	go podInformer.Run(stopCh)
	defer close(stopCh)
	if !cache.WaitForCacheSync(stopCh, podInformer.HasSynced) {
		t.Fatalf("cache not synced")
	}
	for _, tc := range generateRefTestCases {
		conStat := &cgroupStoreManager{
			podInformer: podInformer,
		}
		refData, key, err := conStat.generateRef(tc.cgPath, tc.conStat)
		if err != nil {
			if tc.expect.err == nil {
				t.Fatalf("stats container test case(%s) failed, expect nil err, got err %v",
					tc.describe, err)
			}
			if !strings.Contains(err.Error(), tc.expect.err.Error()) {
				t.Fatalf("stats container test case(%s) failed, expect err %v, got err %v",
					tc.describe, tc.expect.err, err)
			}
			continue
		}
		if tc.expect.err != nil {
			t.Fatalf("stats container test case(%s) failed, expect err %v, got nil err",
				tc.describe, tc.expect.err)
		}

		if !checkRefCgroupEqual(refData, tc.expect.refData) {
			t.Fatalf("stats container test case(%s) failed, expect %+v, got %+v",
				tc.describe, tc.expect.refData, refData)
		}
		if key != tc.expect.key {
			t.Fatalf("stats container test case(%s) failed, expect key %s, got key %s",
				tc.describe, tc.expect.key, key)
		}
	}
}

func checkRefCgroupEqual(a, b *CgroupRef) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name ||
		a.ContainerName != b.ContainerName ||
		a.PodName != b.PodName ||
		a.PodNamespace != b.PodNamespace ||
		a.PodUid != b.PodUid ||
		a.AppClass != b.AppClass {
		return false
	}

	return true
}
