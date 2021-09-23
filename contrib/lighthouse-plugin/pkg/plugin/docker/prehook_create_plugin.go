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
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"

	dockerapi "github.com/docker/docker/client"
	"github.com/docker/docker/runconfig"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
	"github.com/tencent/lighthouse-plugin/pkg/plugin/util"
)

func init() {
	plugin.AllPlugins[VersionedDockerCreatePluginName] = VersionedPrehookCreateContainer
	plugin.AllPlugins[DockerCreatePluginName] = PreHookCreateContainer
}

const (
	// VersionedDockerCreatePluginName is used for compatible with old docker version
	VersionedDockerCreatePluginName = "VersionedDockerPreCreate"
	DockerCreatePluginName          = "DockerPreCreate"
)

type preHookDockerCreatePluginBundle struct {
	plugin.BasePlugin
	startOnce sync.Once
	handlers  []ModifyCreateFunc
	toolKits  *ToolKits
}

type preHookDockerVersionedCreatePluginBundle struct {
	*preHookDockerCreatePluginBundle
}

// ModifyCreateFunc describe handler function type
type ModifyCreateFunc func(
	toolKits *ToolKits,
	containerConfig *runconfig.ContainerConfigWrapper,
	metadata *plugin.PodMetadata) error

var VersionedPrehookCreateContainer = &preHookDockerVersionedCreatePluginBundle{
	preHookDockerCreatePluginBundle: PreHookCreateContainer,
}

var PreHookCreateContainer = &preHookDockerCreatePluginBundle{
	BasePlugin: plugin.BasePlugin{
		Method: http.MethodPost,
		Path:   "/containers/create",
	},
	handlers: make([]ModifyCreateFunc, 0),
}

// Path return URL path with version
func (p *preHookDockerVersionedCreatePluginBundle) Path() string {
	return fmt.Sprintf("/prehook/{id:v[.0-9]+}%s", p.BasePlugin.Path)
}

// SetIgnored set ignored value
func (dcpb *preHookDockerCreatePluginBundle) SetIgnored(ignored plugin.IgnoreNamespacesFunc) {
	dcpb.Ignored = ignored
}

// Method return method name
func (dcpb *preHookDockerCreatePluginBundle) Method() string {
	return dcpb.BasePlugin.Method
}

// Path return URL path
func (dcpb *preHookDockerCreatePluginBundle) Path() string {
	return fmt.Sprintf("/prehook%s", dcpb.BasePlugin.Path)
}

// Handler return handler function, which accept hook request and send to all handlers
func (dcpb *preHookDockerCreatePluginBundle) Handler(k8sClient kubernetes.Interface,
	dockerEndpoint, dockerVersion string) http.HandlerFunc {
	// initialization just do once
	dcpb.startOnce.Do(func() {
		client, err := dockerapi.NewClientWithOpts(dockerapi.WithHost(dockerEndpoint), dockerapi.WithVersion(dockerVersion))
		if err != nil {
			klog.Exitf("can't create docker client, %v", err)
			return
		}

		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{
			Interface: k8sClient.CoreV1().Events(""),
		})

		recorder := eventBroadcaster.NewRecorder(scheme.Scheme,
			v1.EventSource{Component: "plugin-server"})

		// Initialize took kit
		dcpb.toolKits = &ToolKits{
			K8sClient:     k8sClient,
			EventRecorder: record.NewEventRecorderAdapter(recorder),
			DockerClient:  client,
		}
	})

	return func(w http.ResponseWriter, req *http.Request) {
		// accepting hook request
		klog.V(5).Infof("Plugin handle request %s", req.URL.String())
		bodyBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			klog.Errorf("can't read request body: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		retData := &PatchData{
			PatchType: string(types.MergePatchType),
		}
		containerConfig := &runconfig.ContainerConfigWrapper{}

		if err := util.Json.Unmarshal(bodyBytes, &containerConfig); err != nil {
			klog.Errorf("can't unmarshal request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// filter invalid request
		if containerConfig.Labels == nil {
			klog.V(5).Infof("Empty labels")
			util.Json.NewEncoder(w).Encode(retData)
			return
		}

		klog.V(5).Infof("Get pod from container labels")
		metadata := plugin.GetPodMetadata(containerConfig.Labels)
		if dcpb.Ignored(metadata.Namespace) {
			klog.V(5).Infof("Ignored namespace %s", metadata.Namespace)
			util.Json.NewEncoder(w).Encode(retData)
			return
		}

		// send container request to all handlers
		for _, h := range dcpb.handlers {
			if err := h(dcpb.toolKits, containerConfig, metadata); err != nil {
				klog.Errorf("can't handle body, %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		// group final response data
		patchBytes, err := groupPatchData(containerConfig, bodyBytes)
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

// RegisterSubPlugin export register interface
func (dcpb *preHookDockerCreatePluginBundle) RegisterSubPlugin(handler ModifyCreateFunc) {
	dcpb.handlers = append(dcpb.handlers, handler)
}
