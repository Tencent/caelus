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

package dockerclient

import (
	"fmt"
	"github.com/tencent/caelus/pkg/caelus/util/runtime"

	dockertypes "github.com/docker/engine-api/types"
)

type fakeDockerClient struct {
	containers map[string]*dockertypes.ContainerJSON
}

// NewFakeDockerClient create a fake docker client
func NewFakeDockerClient(cons map[string]*dockertypes.ContainerJSON) runtime.RuntimeClient {
	return &fakeDockerClient{
		containers: cons,
	}
}

// InspectContainer inspect container
func (f *fakeDockerClient) InspectContainer(id string) (*dockertypes.ContainerJSON, error) {
	con, ok := f.containers[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return con, nil
}

// ContainerList list containers
func (f *fakeDockerClient) ContainerList(options dockertypes.ContainerListOptions) ([]dockertypes.Container, error) {
	panic("implementing me")
}
