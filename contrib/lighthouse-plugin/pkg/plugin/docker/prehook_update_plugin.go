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
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	dockerapi "github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/util"
)

func init() {
	plugin.AllPlugins[DockerPreHookUpdatePluginName] = PreHookUpdateContainer
}

const (
	DockerPreHookUpdatePluginName = "DockerPreUpdate"
)

type preHookDockerUpdatePlugin struct {
	plugin.BasePlugin
	startOnce sync.Once
	handler   InterceptUpdateFunc
	toolKits  *ToolKits
}

// InterceptUpdateFunc describe handler function type
type InterceptUpdateFunc func(
	toolKits *ToolKits,
	updateConfig *container.UpdateConfig,
	metadata *plugin.PodMetadata,
	containerStatus string) error

var PreHookUpdateContainer = &preHookDockerUpdatePlugin{
	BasePlugin: plugin.BasePlugin{
		Method: http.MethodPost,
		Path:   "/containers/{name:.*}/update",
	},
}

// SetIgnored set ignored value
func (p *preHookDockerUpdatePlugin) SetIgnored(ignored plugin.IgnoreNamespacesFunc) {
	p.Ignored = ignored
}

// Path return URL path
func (p *preHookDockerUpdatePlugin) Path() string {
	return fmt.Sprintf("/prehook%s", p.BasePlugin.Path)
}

// Method return method name
func (p *preHookDockerUpdatePlugin) Method() string {
	return p.BasePlugin.Method
}

// Handler return handler function, which accept hook request and send to all handlers
func (p *preHookDockerUpdatePlugin) Handler(
	k8sClient kubernetes.Interface, dockerEndpoint, dockerVersion string) http.HandlerFunc {
	// initialization just do once
	p.startOnce.Do(func() {
		client, err := dockerapi.NewClientWithOpts(dockerapi.WithHost(dockerEndpoint),
			dockerapi.WithVersion(dockerVersion))
		if err != nil {
			klog.Exitf("can't create docker client, %v", err)
			return
		}

		// initialize tool kit
		p.toolKits = &ToolKits{
			K8sClient:    k8sClient,
			DockerClient: client,
		}
	})

	return func(w http.ResponseWriter, r *http.Request) {
		// accepting hook request
		klog.V(5).Infof("Plugin handle request %s", r.URL.String())
		vars := mux.Vars(r)

		if vars == nil {
			klog.Errorf("can't get vars in request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			klog.Errorf("can't read request body: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		updateConfig := &container.UpdateConfig{}
		if err := util.Json.Unmarshal(bodyBytes, updateConfig); err != nil {
			klog.Errorf("can't unmarshal request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		name := vars["name"]
		klog.V(5).Infof("Update %s container", name)

		// filter invalid request
		metadata, status, err := p.generateContainerData(name, w)
		if err != nil {
			return
		}

		// send docker update request to all handlers
		if err := p.handler(p.toolKits, updateConfig, metadata, status); err != nil {
			klog.Errorf("hook error, %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// group final response data
		patchBytes, err := groupPatchData(updateConfig, bodyBytes)
		if err != nil {
			klog.Errorf("merge patch err: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		klog.V(5).Infof("Docker patch: %s", string(patchBytes))

		util.Json.NewEncoder(w).Encode(&PatchData{
			PatchType: string(types.MergePatchType),
			PatchData: patchBytes,
		})
	}
}

// generateContainerData generate container meta data and status
func (p *preHookDockerUpdatePlugin) generateContainerData(conName string, w http.ResponseWriter) (
	metadata *plugin.PodMetadata, status string, err error) {
	containerJSON, err := p.toolKits.DockerClient.ContainerInspect(context.Background(), conName)
	if err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			klog.Errorf("can't get container json %s, %v", conName, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(noChangeResponse)
		return
	}

	if containerJSON.Config.Labels == nil {
		klog.Warningf("Empty labels")
		w.WriteHeader(http.StatusOK)
		w.Write(noChangeResponse)
		err = fmt.Errorf("empty labels")
		return
	}

	klog.V(5).Infof("Get pod from container labels")
	metadata = plugin.GetPodMetadata(containerJSON.Config.Labels)

	if p.Ignored(metadata.Namespace) {
		klog.V(5).Infof("Ignored namespace %s", metadata.Namespace)
		w.WriteHeader(http.StatusOK)
		w.Write(noChangeResponse)
		err = fmt.Errorf("ignored namespace %s", metadata.Namespace)
		return
	}

	status = containerJSON.State.Status
	return metadata, status, err
}

// RegisterSubPlugin export register interface
func (p *preHookDockerUpdatePlugin) RegisterSubPlugin(h InterceptUpdateFunc) {
	p.handler = h
}
