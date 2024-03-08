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
	"flag"
	"os"
	"path"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/runtime"

	dockerapi "github.com/docker/engine-api/client"
	dockertypes "github.com/docker/engine-api/types"
	"golang.org/x/net/context"
	"k8s.io/klog/v2"
)

var dockerEndPoint = flag.String("container-runtime-endpoint", "unix:///var/run/docker.sock",
	"The endpoint of docker runtime service")
var dockerApiVersion = flag.String("docker-api-version", "v1.32", "The docker api version")

type dockerClient struct {
	client  *dockerapi.Client
	timeout time.Duration
}

func init() {
	// Override runtime endpoint flag if in non-host namespace.
	if _, err := os.Stat(path.Join(types.RootFS, "/proc")); err == nil {
		flagOverrides := map[string]string{
			"container-runtime-endpoint": "unix://" + types.RootFS + "/var/run/docker.sock",
		}
		for name, defaultValue := range flagOverrides {
			if f := flag.Lookup(name); f != nil {
				f.DefValue = defaultValue
				f.Value.Set(defaultValue)
			} else {
				klog.Errorf("Expected runtime endpoint flag %q not found", name)
			}
		}
	}
}

// NewDockerClient create docker runtime client
func NewDockerClient() runtime.RuntimeClient {
	klog.Infof("connecting to docker on %s", *dockerEndPoint)
	client, err := dockerapi.NewClient(*dockerEndPoint, *dockerApiVersion, nil, nil)
	if err != nil {
		klog.Fatalf("connecting docker failed on %s: %v", *dockerEndPoint, err)
	}

	return &dockerClient{
		client:  client,
		timeout: time.Duration(30 * time.Second),
	}
}

func (d *dockerClient) getTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d.timeout)
}

// InspectContainer inspect docker container
func (d *dockerClient) InspectContainer(id string) (*dockertypes.ContainerJSON, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	containerJSON, err := d.client.ContainerInspect(ctx, id)
	if err := hasError(ctx, err); err != nil {
		return nil, err
	}
	return &containerJSON, nil
}

// ContainerList list docker containers
func (d *dockerClient) ContainerList(options dockertypes.ContainerListOptions) ([]dockertypes.Container, error) {
	ctx, cancel := d.getTimeoutContext()
	defer cancel()
	containers, err := d.client.ContainerList(ctx, options)
	if err := hasError(ctx, err); err != nil {
		return containers, err
	}

	return containers, nil
}

// hasError check the context firstly, and return error if the context has error,
// then it check if err is nil.
func hasError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	return err
}
