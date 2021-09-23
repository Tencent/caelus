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

package plugin

import (
	"net/http"

	"k8s.io/apimachinery/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes"
	"k8s.io/component-base/featuregate"
)

// Plugin describe common functions
type Plugin interface {
	Method() string
	Path() string
	Handler(client kubernetes.Interface, dockerEndpoint, dockerVersion string) http.HandlerFunc
	SetIgnored(IgnoreNamespacesFunc)
}

type IgnoreNamespacesFunc func(string) bool

// BasePlugin describe common variables
type BasePlugin struct {
	Method  string
	Path    string
	Ignored IgnoreNamespacesFunc
	Handler http.HandlerFunc
}

var (
	AllPlugins = make(map[string]Plugin)
)

const (
	ContainerTypeLabelKey       = "io.kubernetes.docker.type"
	ContainerTypeLabelContainer = "container"
	ContainerSandBoxKey         = "io.kubernetes.sandbox.id"
	ContainerNameLabelKey       = "io.kubernetes.container.name"
	PodNamespaceLabelKey        = "io.kubernetes.pod.namespace"
	PodNameLabelKey             = "io.kubernetes.pod.name"
	PodUIDLabelKey              = "io.kubernetes.pod.uid"
	// sandbox container will add prefix with value "annotation."
	PodAutoAnnotationPrefix = "annotation."
	PodAnnotationPrefix     = PodAutoAnnotationPrefix + "mixer.kubernetes.io/"
)

const (
	// docker storage option
	DockerStorageOption featuregate.Feature = "DockerStorageOption"
	// UTS mode support, create a new UTS namespace different with host namespace
	DockerUTSMode featuregate.Feature = "DockerUTSMode"
	// Offline support, create cgroup path different with kubernetes
	DockerOfflineMutate featuregate.Feature = "DockerOfflineMutate"
	// limit pod pid number
	DockerPidsLimit featuregate.Feature = "DockerPidsLimit"
)

// feature gate support
var defaultFeatureGate = map[featuregate.Feature]featuregate.FeatureSpec{
	DockerStorageOption: {Default: false, PreRelease: featuregate.Alpha},
	DockerUTSMode:       {Default: false, PreRelease: featuregate.Alpha},
	DockerOfflineMutate: {Default: false, PreRelease: featuregate.Alpha},
	DockerPidsLimit:     {Default: false, PreRelease: featuregate.Alpha},
}

func init() {
	runtime.Must(utilfeature.DefaultMutableFeatureGate.Add(defaultFeatureGate))
}
