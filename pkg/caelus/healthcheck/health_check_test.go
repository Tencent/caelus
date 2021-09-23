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

package health

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
)

var (
	testNodeName = "testNode"
	testHash     = "9589f334c6f4987fc5ddb8e0ac1c096b"
)

// TestNeedReload test if the rule config need to reload
func TestNeedReload(t *testing.T) {
	configFile := "/tmp/testing"
	err := ioutil.WriteFile(configFile, []byte("just testing"), 0644)
	if err != nil {
		t.Skipf("creating testing file %s err: %v", configFile, err)
	}
	defer os.Remove(configFile)

	healthManager := &manager{
		configHash: "123",
		configUpdateFunc: func(string) (*types.HealthCheckConfig, error) {
			return &types.HealthCheckConfig{
				Disable:   false,
				RuleNodes: []string{testNodeName},
			}, nil
		},
	}

	util.SetNodeIP(testNodeName)
	reload, hash, _ := healthManager.checkNeedReload(configFile)
	if !reload || hash != testHash {
		t.Fatalf("health check manager reload not expected, got reload:%v, hash:%s, expect reload:true, hash:%s",
			reload, hash, testHash)
	}
}
