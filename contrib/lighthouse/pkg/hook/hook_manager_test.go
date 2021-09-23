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

package hook

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tencent/lighthouse/pkg/apis/componentconfig.lighthouse.io/v1alpha1"
	"github.com/tencent/lighthouse/pkg/test"
)

func TestHookManagerPreHook(t *testing.T) {
	flag.Set("v", "4")
	flag.Parse()

	testUnits := []*testHookManagerUnit{
		{
			path:     fmt.Sprintf("/container/%s/create", uuid.New().String()),
			pattern:  "/container/{id:[-a-z0-9]+}/create",
			payload:  `{"foo":"bar"}`,
			expected: `{"a":"b"}`,
			patches: []*PatchData{
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"replace","path":"/foo", "value":"1"}]`),
				},
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"remove","path":"/foo"}]`),
				},
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"add","path":"/a","value":"b"}]`),
				},
			},
		},
		{
			path:     fmt.Sprintf("/container/%s/create", uuid.New().String()),
			pattern:  "/container/{id:[0-9]+}/create}",
			payload:  `{"foo":"bar"}`,
			expected: `{"foo":"bar"}`,
			patches: []*PatchData{
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"replace","path":"/foo", "value":"1"}]`),
				},
			},
		},
	}

	for i := range testUnits {
		func() {
			u := testUnits[i]
			backendServer := createTestServerBundle(1)
			hookServer := createTestServerBundle(len(u.patches))

			for j := range u.patches {
				p := u.patches[j]
				hookServer.servers[j].RegisterHandler(HookPath(v1alpha1.PreHookType, u.path), func(w http.ResponseWriter,
					r *http.Request) {
					json.NewEncoder(w).Encode(p)
				})
			}

			backendServer.servers[0].RegisterHandler(u.path, func(w http.ResponseWriter, r *http.Request) {
				bodyBytes, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Errorf("can't get body: %v", err)
					return
				}

				if string(bodyBytes) != u.expected {
					t.Errorf("%d expected: %s to be %s", i, string(bodyBytes), u.expected)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			cfg := newHookConfiguration(backendServer.servers[0].GetAddress(), len(u.patches),
				u.patches, u.pattern, hookServer.servers, v1alpha1.PreHookType)

			hookManagerRunningCheck(cfg, backendServer, hookServer, u.payload, u.path, t)
		}()
	}
}

func TestHookManagerPostHook(t *testing.T) {
	flag.Set("v", "4")
	flag.Parse()

	testUnits := []*testHookManagerUnit{
		{
			path:     fmt.Sprintf("/container/%s/create", uuid.New().String()),
			pattern:  "/container/{id:[-a-z0-9]+}/create",
			payload:  `{"foo":"bar"}`,
			expected: `{"a":"b"}`,
			patches: []*PatchData{
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"replace","path":"/body/foo", "value":"1"}]`),
				},
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"remove","path":"/body/foo"}]`),
				},
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"add","path":"/body/a","value":"b"}]`),
				},
			},
		},
		{
			path:     fmt.Sprintf("/container/%s/create", uuid.New().String()),
			pattern:  "/container/{id:[0-9]+}/create}",
			payload:  `{"foo":"bar"}`,
			expected: `{"foo":"bar"}`,
			patches: []*PatchData{
				{
					PatchType: string(types.JSONPatchType),
					PatchData: []byte(`[{"op":"replace","path":"/body/foo", "value":"1"}]`),
				},
			},
		},
	}

	for i := range testUnits {
		func() {
			u := testUnits[i]
			backendServer := createTestServerBundle(1)
			hookServer := createTestServerBundle(len(u.patches))

			for j := range u.patches {
				p := u.patches[j]
				hookServer.servers[j].RegisterHandler(HookPath(v1alpha1.PostHookType, u.path), func(w http.ResponseWriter,
					r *http.Request) {
					json.NewEncoder(w).Encode(p)
				})
			}

			backendServer.servers[0].RegisterHandler(u.path, func(w http.ResponseWriter, r *http.Request) {
				bodyBytes, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Errorf("can't get body: %v", err)
					return
				}

				if string(bodyBytes) != u.payload {
					t.Errorf("expected: %s to be %s", string(bodyBytes), u.expected)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(bodyBytes)
			})

			cfg := newHookConfiguration(backendServer.servers[0].GetAddress(), len(u.patches),
				u.patches, u.pattern, hookServer.servers, v1alpha1.PostHookType)

			hookManagerRunningCheck(cfg, backendServer, hookServer, u.payload, u.path, t)
		}()
	}
}

func newHookConfiguration(remoteEndpoint string, hookLength int,
	patches []*PatchData, pattern string, servers []*test.UnixSocketServer, hookType v1alpha1.HookType) *v1alpha1.HookConfiguration {
	cfg := &v1alpha1.HookConfiguration{
		Timeout:        10,
		ListenAddress:  fmt.Sprintf("unix://@%s", uuid.New().String()),
		RemoteEndpoint: remoteEndpoint,
		WebHooks:       make(v1alpha1.HookConfigurationList, hookLength),
	}

	for j := range patches {
		webhook := &cfg.WebHooks[j]

		webhook.Endpoint = servers[j].GetAddress()
		webhook.Name = fmt.Sprintf("hook-%d", j)
		webhook.FailurePolicy = v1alpha1.PolicyFail
		webhook.Stages = append(webhook.Stages, v1alpha1.HookStage{
			Method:     http.MethodPost,
			URLPattern: pattern,
			Type:       hookType,
		})
	}

	return cfg
}

func hookManagerRunningCheck(cfg *v1alpha1.HookConfiguration, backendServer, hookServer *testServerBundle,
	payload, path string, t *testing.T) {
	hm := NewHookManager()
	if err := hm.InitFromConfig(cfg); err != nil {
		t.Errorf("can't init hook manager: %v", err)
		return
	}

	totalServerNum := len(backendServer.servers) + len(hookServer.servers)
	readyCh := make(chan bool, totalServerNum)
	go func() {
		backendServer.Start(readyCh)
	}()

	go func() {
		hookServer.Start(readyCh)
	}()

	defer func() {
		backendServer.Stop()
		hookServer.Stop()
	}()

	for len(readyCh) != totalServerNum {
		time.Sleep(time.Second)
	}

	ans := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s", cfg.ListenAddress),
		bytes.NewBuffer([]byte(payload)))
	if err != nil {
		t.Errorf("can't create HTTP request: %v", err)
		return
	}
	req.URL.Path = path

	hm.ServeHTTP(ans, req)

	resp := ans.Result()
	if resp == nil {
		t.Errorf("resp is nil")
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected %d to be %d", resp.StatusCode, http.StatusOK)
		return
	}
}

type testServerBundle struct {
	servers []*test.UnixSocketServer
}

func createTestServerBundle(num int) *testServerBundle {
	b := &testServerBundle{
		servers: make([]*test.UnixSocketServer, num),
	}

	for i := 0; i < num; i++ {
		b.servers[i] = test.NewUnixSocketServer()
	}

	return b
}

func (b *testServerBundle) Start(ready chan bool) error {
	ch := make(chan error, len(b.servers))

	for i := range b.servers {
		s := b.servers[i]
		go func() {
			ready <- true
			ch <- s.Start()
		}()
	}

	select {
	case e := <-ch:
		return e
	}
}

func (b *testServerBundle) Stop() {
	for _, s := range b.servers {
		s.Stop()
	}
}

type testHookManagerUnit struct {
	path     string
	pattern  string
	payload  string
	expected string
	patches  []*PatchData
}
