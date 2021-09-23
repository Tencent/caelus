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

package hadoop

import (
	"strings"
	"testing"

	"gotest.tools/assert"
)

const YARN_SITE = `<?xml version="1.0"?>
<configuration>
<property>
        <name>yarn.resourcemanager.address</name>
        <value>RM_IP:18032</value>
</property>
<property>
        <name>yarn.resourcemanager.resource-tracker.address</name>
        <value>RM_IP:18031</value>
</property>
<property>
        <name>yarn.resourcemanager.webapp.address</name>
        <value>RM_IP:8080</value>
</property>
<property>
        <name>yarn.resourcemanager.scheduler.address</name>
        <value>RM_IP:18030</value>
</property>

<property>
        <name>yarn.nodemanager.resource.memory-mb</name>
        <value>MEMORY</value>
</property>

<property>
        <name>yarn.nodemanager.resource.cpu-vcores</name>
        <value>VCORE</value>
</property>
<property>
        <name>yarn.nodemanager.local-dirs</name>
	<value>/data/nm-local</value>
</property>

<property>
        <name>yarn.nodemanager.log-dirs</name>
	<value>/data/nm-log</value>
</property>
</configuration>`

// TestGetConfDataFromStream test if to get xml data from io reader
func TestGetConfDataFromStream(t *testing.T) {
	reader := strings.NewReader(YARN_SITE)
	confData, err := LoadConfDataFromStream(reader)
	if err != nil {
		t.Fail()
	}

	assert.Equal(t, "MEMORY", confData.Get("yarn.nodemanager.resource.memory-mb"))
	assert.Equal(t, "VCORE", confData.Get("yarn.nodemanager.resource.cpu-vcores"))
	assert.Equal(t, confData.Get("xxx"), "")
}

// TestSetConfDataFromStream test if to get xml data from io reader
func TestSetConfDataFromStream(t *testing.T) {
	reader := strings.NewReader(YARN_SITE)
	confData, err := LoadConfDataFromStream(reader)
	if err != nil {
		t.Fail()
	}

	assert.Equal(t, "MEMORY", confData.Get("yarn.nodemanager.resource.memory-mb"))
	confData.Set("yarn.nodemanager.resource.memory-mb", "1024")
	assert.Equal(t, "1024", confData.Get("yarn.nodemanager.resource.memory-mb"))
}
