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
	"github.com/tencent/caelus/pkg/version/verflag"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

// options describe supported flags
type options struct {
	ApiOption ApiOption
}

// printFlags show all flags
func printFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})
}

// NewServerCommand construct server execution context
func NewServerCommand() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use: "nm_operator",
		Run: func(cmd *cobra.Command, args []string) {
			verflag.PrintAndExitIfRequested()
			printFlags(cmd.Flags())

			if err := opts.Run(); err != nil {
				klog.Exitf("can't run command, %v", err)
			}
		},
	}

	opts.AddFlags(cmd.Flags())

	return cmd
}

// AddFlags describe supported flags
func (o *options) AddFlags(fs *pflag.FlagSet) {
	o.ApiOption.AddFlags(fs)
}

// Run starts the main loop
func (o *options) Run() error {
	if err := o.ApiOption.RegisterServerAndListen(); err != nil {
		return err
	}

	return nil
}
