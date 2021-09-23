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

package machine

import (
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	"k8s.io/klog"
)

const NET_SPEED = "NET_SPEED"

var (
	// accessing the map is not very frequency, no need to add lock
	devSpeedMap = make(map[string]int)
)

// GetIfaceSpeed get speed of ifName from /sys/class/net/$ifName/speed, or environment
func GetIfaceSpeed(ifName string) int {
	if v, ok := devSpeedMap[ifName]; ok {
		return v
	}

	speed, err := getSpeed(ifName)
	if err != nil {
		klog.Errorf("get speed of %s from local file err: %v", ifName, err)
	}
	if speed <= 0 || speed > 100*1000 { // assume max speed is 100Gbit
		speed, _ = strconv.Atoi(os.Getenv(NET_SPEED))
	}
	if speed <= 0 {
		klog.Fatalf("bad speed of %s %d", ifName, speed)
	}
	devSpeedMap[ifName] = speed
	return speed
}

// getSpeed get speed of ifName from /sys/class/net/$ifName/speed
func getSpeed(ifName string) (int, error) {
	file := path.Join("/sys/class/net/", ifName, "/speed")
	speedStr, err := ioutil.ReadFile(file)
	if err != nil {
		return 0, err
	}
	value := strings.Replace(string(speedStr), "\n", "", -1)
	return strconv.Atoi(value)
}
