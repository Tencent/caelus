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

package v1alpha1

import (
	"net/http"
)

// SetDefaults_HookConfiguration set default value for hook configuration
func SetDefaults_HookConfiguration(obj *HookConfiguration) {
	if obj.Timeout == 0 {
		obj.Timeout = 5
	}

	if obj.RemoteEndpoint == "" {
		obj.RemoteEndpoint = "unix:///var/run/docker.sock"
	}
}

// SetDefaults_HookConfigurationItem set default value for HookConfigurationItem
func SetDefaults_HookConfigurationItem(obj *HookConfigurationItem) {
	if obj.FailurePolicy == "" {
		obj.FailurePolicy = PolicyFail
	}
}

// SetDefaults_HookStage set default value for HookStage
func SetDefaults_HookStage(obj *HookStage) {
	if obj.Method == "" {
		obj.Method = http.MethodPost
	}
}
