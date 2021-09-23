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
	"encoding/json"
	"io/ioutil"
	"k8s.io/klog"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"
	"github.com/tencent/caelus/pkg/metricadapter"
	"github.com/tencent/caelus/pkg/version/verflag"
)

const (
	// config file, now just support reading from fixed file name
	configFile = "/etc/caelus/adapter.json"
)

// metric_adapter is used to collect outer metrics to caelus,
// such as online jobs' latency metrics
func main() {
	flag.Parse()
	verflag.PrintAndExitIfRequested()
	config := getConfig()
	manager := metricadapter.NewAdapterManager(config)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	manager.Run(sig)
}

// getConfig read config file from fixed file name
func getConfig() metricadapter.AdapterConfig {
	configBytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		klog.Fatalf("config error: %v", err)
	}
	config := metricadapter.AdapterConfig{}
	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		klog.Fatalf("config error: %v", err)
	}
	return config
}
