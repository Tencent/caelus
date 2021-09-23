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
	"context"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/klog"

	"github.com/tencent/lighthouse/pkg/apis/componentconfig.lighthouse.io/v1alpha1"
	"github.com/tencent/lighthouse/pkg/util"
)

// hookerConnector used to forward docker request to backend
type hookerConnector struct {
	name          string
	endpoint      string
	failurePolicy v1alpha1.FailurePolicyType
	client        *http.Client
}

var _ HookHandler = &hookerConnector{}

func newHookConnector(name, endpoint string, failurePolicy v1alpha1.FailurePolicyType) *hookerConnector {
	hc := &hookerConnector{
		name:          name,
		endpoint:      endpoint,
		failurePolicy: failurePolicy,
	}

	// strict endpoint address check
	if !strings.HasPrefix(endpoint, "unix://") {
		klog.Fatalf("only support unix protocol")
	}

	// init backend client
	hc.client = util.BuildClientOrDie(endpoint)
	return hc
}

// performHook forward docker request to backend
func (hc *hookerConnector) performHook(ctx context.Context, patch *PatchData, method, path string, body []byte) error {
	url := fmt.Sprintf("http://%s", hc.endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil && !hc.allowFailure() {
		klog.Errorf("can't create request %s, %v", url, err)
		return err
	}

	if req != nil {
		req.URL.Path = path

		klog.V(4).Infof("Send request %s %s for %s", method, path, hc.name)
		resp, err := hc.client.Do(req)
		if err != nil && !hc.allowFailure() {
			return err
		}

		if resp != nil {
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && !hc.allowFailure() {
				return fmt.Errorf("post is not success, status code is %d", resp.StatusCode)
			}

			if resp.Body != nil {
				klog.V(4).Infof("Decode response %s for %s", path, hc.name)
				return json.NewDecoder(resp.Body).Decode(patch)
			}
		}
	}

	return nil
}

// PreHook do pre hook type request to backend server
func (hc *hookerConnector) PreHook(ctx context.Context, patch *PatchData, method, path string, body []byte) error {
	return hc.performHook(ctx, patch, method, HookPath(v1alpha1.PreHookType, path), body)
}

// PostHook do post hook type request to backend server
func (hc *hookerConnector) PostHook(ctx context.Context, patch *PatchData, method, path string, body []byte) error {
	return hc.performHook(ctx, patch, method, HookPath(v1alpha1.PostHookType, path), body)
}

// allowFailure check if failed based on failure policy
func (hc *hookerConnector) allowFailure() bool {
	return hc.failurePolicy == v1alpha1.PolicyIgnore
}
