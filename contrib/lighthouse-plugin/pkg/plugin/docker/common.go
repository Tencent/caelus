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

package docker

import (
	gjson "encoding/json"
	"fmt"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/util"

	dockerapi "github.com/docker/docker/client"
	"github.com/evanphx/json-patch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/events"
)

var (
	noChangeResponse = []byte(`{}`)
)

// ToolKits group some clients
type ToolKits struct {
	K8sClient     kubernetes.Interface
	EventRecorder events.EventRecorder
	DockerClient  *dockerapi.Client
}

// PatchData, copied from lighthouse/pkg/hook/types.go
type PatchData struct {
	PatchType string `json:"patchType,omitempty"`
	PatchData []byte `json:"patchData,omitempty"`
}

// PostHookData, copied from lighthouse/pkg/hook/types.go
type PostHookData struct {
	StatusCode int              `json:"statusCode,omitempty"`
	Body       gjson.RawMessage `json:"body,omitempty"`
}

func groupPatchData(config interface{}, bodyBytes []byte) ([]byte, error) {
	newBodyBytes, err := util.Json.Marshal(config)
	if err != nil {
		return []byte{}, fmt.Errorf("cannit marshal, :%v", err)
	}

	patchBytes, err := jsonpatch.CreateMergePatch(bodyBytes, newBodyBytes)
	if err != nil {
		return []byte{}, fmt.Errorf("can't create patch, %v", err)
	}

	return patchBytes, nil
}
