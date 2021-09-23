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

package appclass

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var appClassCases = []struct {
	describe  string
	pod       *v1.Pod
	expect    AppClass
	isOffline bool
}{
	{
		describe: "test " + AppClassSystem,
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "kube-system",
			},
		},
		expect:    AppClassSystem,
		isOffline: false,
	},
	{
		describe: "test " + AppClassUnknown,
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{},
		},
		expect:    AppClassUnknown,
		isOffline: false,
	},
	{
		describe: "test " + AppClassOffline,
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnotationOfflineKey: AnnotationOfflineValue,
				},
			},
		},
		expect:    AppClassOffline,
		isOffline: true,
	},
	{
		describe: "test " + AppClassOnline,
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Annotations: map[string]string{
					AnnotationOfflineKey: "online",
				},
			},
		},
		expect:    AppClassOnline,
		isOffline: false,
	},
}

// TestGetAppClass tests app class
func TestGetAppClass(t *testing.T) {
	for _, ac := range appClassCases {
		t.Logf("start case: %s", ac.describe)
		result := GetAppClass(ac.pod)
		if result != ac.expect {
			t.Fatalf("get app class error, expect %s, but get %s", ac.expect, result)
		}
	}
}

// TestIsOffline tests if the app is offline
func TestIsOffline(t *testing.T) {
	for _, ac := range appClassCases {
		t.Logf("start case: %s", ac.describe)
		result := IsOffline(ac.pod)
		if result != ac.isOffline {
			t.Fatalf("offline check err, expect %v, but get %v", ac.isOffline, result)
		}
	}
}
