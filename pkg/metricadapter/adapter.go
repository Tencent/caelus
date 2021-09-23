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

package metricadapter

import (
	"encoding/json"
	"fmt"
	"k8s.io/klog"
	"net/http"
	"os"
)

// adapter define functions for metric adapter
type adapter interface {
	Run(stopCh <-chan struct{})
	GetSlo() []podMetric
}

// Manager group multiple adapters
type Manager struct {
	port     int
	adapters []adapter
}

// NewAdapterManager create adapter manager
func NewAdapterManager(config AdapterConfig) *Manager {
	var adapters []adapter
	adapters = append(adapters, newServerAdapter())
	if config.ServingPort <= 0 {
		config.ServingPort = defaultServingPort
	}
	return &Manager{adapters: adapters, port: config.ServingPort}
}

// Run start to run metric adapter manager
func (m *Manager) Run(sig <-chan os.Signal) {
	stopCh := make(chan struct{})
	for _, adapter := range m.adapters {
		adapter.Run(stopCh)
	}
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", m.port), nil)
		if err != nil {
			klog.Fatalf("failed listen server_adapter: %v", err)
		}
	}()
	<-sig
	close(stopCh)
	return
}

func (m *Manager) setupSloHandler() {
	http.HandleFunc("/metricdata", func(writer http.ResponseWriter, request *http.Request) {
		var res podMetrics
		for _, adapter := range m.adapters {
			slos := adapter.GetSlo()
			res.Data = append(res.Data, slos...)
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			klog.Errorf("failed marshaling metrics: %v", err)
		}
		writer.Write(resBytes)
	})
}
