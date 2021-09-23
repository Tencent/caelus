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
	"fmt"
	"io/ioutil"

	"github.com/mYmNeo/version/verflag"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	"github.com/tencent/lighthouse/pkg/apis/componentconfig.lighthouse.io/v1alpha1"
	"github.com/tencent/lighthouse/pkg/hook"
	"github.com/tencent/lighthouse/pkg/scheme"
	"github.com/tencent/lighthouse/pkg/util"
)

// Options group server options
type Options struct {
	ConfigFile string
	config     *v1alpha1.HookConfiguration
}

// NewLighthouseCommand construct server execution context
func NewLighthouseCommand() *cobra.Command {
	opts := NewOptions()

	cmd := &cobra.Command{
		Use: "lighthouse",
		Long: "The lighthouse runs on each node. This is a preHook framework to modify request body for " +
			"any matched rules in the configuration. It is an enhancement for kubelet to run a container",
		Run: func(cmd *cobra.Command, args []string) {
			verflag.PrintAndExitIfRequested()
			util.PrintFlags(cmd.Flags())

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
	return &Options{
		config: &v1alpha1.HookConfiguration{},
	}
}

// Run main loop
func (o *Options) Run() error {
	hookServer := hook.NewHookManager()

	if err := hookServer.InitFromConfig(o.config); err != nil {
		return err
	}

	return hookServer.Run(context.Background().Done())
}

// AddFlags describe supported flags
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ConfigFile, "config", o.ConfigFile, "The path to the configuration file")
}

// Complete is mainly for parsing config file
func (o *Options) Complete() error {
	if len(o.ConfigFile) > 0 {
		cfgData, err := ioutil.ReadFile(o.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to read hook configuration file %q, %v", o.ConfigFile, err)
		}

		// decode hook configuration
		versioned := &v1alpha1.HookConfiguration{}
		v1alpha1.SetObjectDefaults_HookConfiguration(versioned)
		decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion)
		if err := runtime.DecodeInto(decoder, cfgData, versioned); err != nil {
			return fmt.Errorf("failed to decode hook configuration file %q, %v", o.ConfigFile, err)
		}

		// convert versioned hook configuration to internal version
		if err := scheme.Scheme.Convert(versioned, o.config, nil); err != nil {
			return fmt.Errorf("failed to convert versioned hook configurtion to internal version, %v", err)
		}
	}
	return nil
}
