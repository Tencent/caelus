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

package app

import (
	"context"
	"strings"

	"github.com/mYmNeo/version/verflag"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"

	"github.com/tencent/lighthouse-plugin/pkg/server"
)

// Options group server options
type Options struct {
	Master            string
	HostnameOverride  string
	Kubeconfig        string
	ListenAddress     string
	FeatureGates      map[string]bool
	DockerEndpoint    string
	DockerVersion     string
	IgnoredNamespaces []string
}

// NewPluginServerCommand construct server execution context
func NewPluginServerCommand() *cobra.Command {
	opts := NewOptions()

	cmd := &cobra.Command{
		Use:  "plugin-server",
		Long: "The plugin server provides hook request send to runtime backend",
		Run: func(cmd *cobra.Command, args []string) {
			// version flag supported, like --version""
			verflag.PrintAndExitIfRequested()
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})

			if err := opts.Complete(); err != nil {
				klog.Fatalf("failed complete: %v", err)
			}

			if err := opts.Run(); err != nil {
				klog.Exit(err)
			}
		},
	}

	opts.AddFlags(cmd.Flags())

	return cmd
}

// NewOptions return option instance
func NewOptions() *Options {
	// fixed version, you may adapt it to your target version
	return &Options{
		DockerEndpoint: "unix:///var/run/docker.sock",
		DockerVersion:  "1.38",
	}
}

// AddFlags describe supported flags
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "The path of kubernetes config to communicate with Kubernetes")
	fs.StringVar(&o.ListenAddress, "listen-address", o.ListenAddress, "The listen address of plugin server")
	fs.StringVar(&o.HostnameOverride, "hostname-override", o.HostnameOverride,
		"If non-empty, will use this string as identification instead of the actual hostname.")
	fs.StringVar(&o.Master, "master", o.Master, "apiserver master address")
	fs.Var(cliflag.NewMapStringBool(&o.FeatureGates), "feature-gates",
		"A set of key=value pairs that describe feature gates for alpha/experimental features. Options are:\n"+
			strings.Join(utilfeature.DefaultMutableFeatureGate.KnownFeatures(), "\n"))
	fs.StringVar(&o.DockerEndpoint, "docker-endpoint", o.DockerEndpoint, "docker endpoint address")
	fs.StringVar(&o.DockerVersion, "docker-version", o.DockerVersion, "docker client version")
	fs.StringSliceVar(&o.IgnoredNamespaces, "ignored-namespaces", o.IgnoredNamespaces, "The containers with the specific"+
		"namespaces to be ignored")
}

// Complete describe feature gate flags
func (o *Options) Complete() error {
	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(o.FeatureGates); err != nil {
		return err
	}
	return nil
}

// Run main loop
func (o *Options) Run() error {
	// build kubernetes client
	kubeconfig, err := clientcmd.BuildConfigFromFlags(o.Master, o.Kubeconfig)
	if err != nil {
		return err
	}
	// init plugin server
	pluginServer := server.NewPluginServer(o.ListenAddress, clientset.NewForConfigOrDie(kubeconfig),
		o.DockerEndpoint, o.DockerVersion, o.IgnoredNamespaces)

	return pluginServer.Run(context.Background().Done())
}
