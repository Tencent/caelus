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

package predict

import (
	"fmt"
	"net/http"
	"time"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"github.com/parnurzeal/gorequest"
	"k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	PredictPath = "/v1/predict/online/"
)

// vpaPredict describe options for VPA predict
type vpaPredict struct {
	types.PredictConfig
	rpcClient *gorequest.SuperAgent
}

// AddSample starts add sample data periodically
func (p *vpaPredict) AddSample(stop <-chan struct{}) {
	// nothing need to do
}

// Predict predicts resources used by online pods
func (p *vpaPredict) Predict() v1.ResourceList {
	res := v1.ResourceList{}
	nodeName := util.NodeName()

	// get predict result from remote VPA server
	client := p.rpcClient.Clone().Get(fmt.Sprintf("http://%s%s%s",
		p.PredictServerAddr, PredictPath, nodeName))
	resp, _, errs := client.EndStruct(&res)
	if len(errs) > 0 {
		klog.Errorf("can't get node resource list, %v", errs[0])
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("can't get node resource list, status code: %d", resp.StatusCode)
		return nil
	}
	return res
}

// NewVpaPredictOrDie creates a vpa predictor
func NewVpaPredictOrDie(config types.PredictConfig) Predictor {
	return &vpaPredict{
		PredictConfig: config,
		rpcClient:     gorequest.New().SetDebug(bool(klog.V(4).Enabled())).Timeout(time.Second * 10),
	}
}
