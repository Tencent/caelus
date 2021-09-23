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
	"strings"

	jsoniter "github.com/json-iterator/go"

	"github.com/tencent/lighthouse/pkg/apis/componentconfig.lighthouse.io/v1alpha1"
)

var json = jsoniter.Config{
	EscapeHTML:             false,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
}.Froze()

// HookPath group hook request path
func HookPath(hookType v1alpha1.HookType, path string) string {
	return strings.ToLower(strings.Join([]string{"/", string(hookType), path}, ""))
}

// fixUnexpectedEscape fix Escape error
func fixUnexpectedEscape(d []byte) []byte {
	return []byte(strings.ReplaceAll(strings.ReplaceAll(string(d), `\u003c`, "<"), `\u003e`, ">"))
}
