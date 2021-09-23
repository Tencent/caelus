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

package main

import (
	goflag "flag"
	"math/rand"
	"os"
	"time"

	"github.com/tencent/caelus/cmd/nm-operator/app"
	"github.com/tencent/caelus/pkg/version/verflag"

	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
)

// This is the main function for nm_operator server
func main() {
	rand.Seed(time.Now().UnixNano())
	// init command context
	command := app.NewServerCommand()

	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	logs.InitLogs()
	defer logs.FlushLogs()

	// support version flag, such as "--version"
	verflag.PrintAndExitIfRequested()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
