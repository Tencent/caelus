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
	"context"
	"flag"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	"github.com/tencent/lighthouse/pkg/apis/componentconfig.lighthouse.io/v1alpha1"
	"github.com/tencent/lighthouse/pkg/test"
)

func init() {
	klog.InitFlags(nil)
}

func TestHookConnectorPreHook(t *testing.T) {
	flag.Set("v", "4")
	flag.Parse()

	testUnits := []*testConnectorUnit{
		{
			patch: &PatchData{
				PatchType: string(types.JSONPatchType),
				PatchData: []byte(`[{"op":"replace", "path":"/foo", "value":"bar"}]`),
			},
			payload: `{"foo":1}`,
			path:    "/replace",
		},
		{
			patch: &PatchData{
				PatchType: string(types.MergePatchType),
				PatchData: []byte(`{}`),
			},
			payload: `{"foo":"bar"}`,
			path:    "/merge",
		},
	}
	server := test.NewUnixSocketServer()

	for i := range testUnits {
		u := testUnits[i]
		path := HookPath(v1alpha1.PreHookType, u.path)
		server.RegisterHandler(path, func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				t.Errorf("expect request method %s to be %s", req.Method, http.MethodPost)
				return
			}

			bodyBytes, err := ioutil.ReadAll(req.Body)
			if err != nil {
				t.Errorf("can't read body, %v", err)
				return
			}

			if u.payload != string(bodyBytes) {
				t.Errorf("expect payload %s to be %s", string(bodyBytes), u.payload)
				return
			}

			json.NewEncoder(w).Encode(u.patch)
		})
	}

	ready := make(chan struct{})
	go func() {
		close(ready)
		server.Start()
	}()

	defer server.Stop()
	<-ready

	hc := newHookConnector("test", server.GetAddress(), v1alpha1.PolicyFail)

	for _, u := range testUnits {
		p := &PatchData{}
		if err := hc.PreHook(context.Background(), p, http.MethodPost, u.path, []byte(u.payload)); err != nil {
			t.Errorf("can't perform a hook, %v", err)
			return
		}

		if !reflect.DeepEqual(p, u.patch) {
			t.Errorf("expected %+#v, got %+#v", p, u.patch)
			return
		}
	}
}

type testConnectorUnit struct {
	patch   *PatchData
	path    string
	payload string
}
