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
	gjson "encoding/json"
	"net/http"
	"net/http/httptest"
)

// PatchData show struct data for patch method
type PatchData struct {
	PatchType string `json:"patchType,omitempty"`
	PatchData []byte `json:"patchData,omitempty"`
}

// PostHookData show response data for post hook
type PostHookData struct {
	StatusCode int              `json:"statusCode,omitempty"`
	Body       gjson.RawMessage `json:"body,omitempty"`
}

// HookHandler describe hook interface
type HookHandler interface {
	PreHook(ctx context.Context, patch *PatchData, method, path string, body []byte) error
	PostHook(ctx context.Context, patch *PatchData, method, path string, body []byte) error
}

type PreHookFunc func(w http.ResponseWriter, r *http.Request) error
type PostHookFunc func(w *httptest.ResponseRecorder, r *http.Request)
