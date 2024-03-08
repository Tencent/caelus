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

package netio

import (
	"github.com/chenchun/ipset/log"
	"k8s.io/klog/v2"
)

var _ log.LOG = ipsetLog{}

// ipsetLog is the log implementation of ipset.Log
type ipsetLog struct {
}

// Debugf prints debug log
func (i ipsetLog) Debugf(format string, args ...interface{}) {
	klog.V(4).Infof(format, args...)
}

// Infof prints info log
func (i ipsetLog) Infof(format string, args ...interface{}) {
	klog.Infof(format, args...)
}

// Fatalf prints fatal log
func (i ipsetLog) Fatalf(format string, args ...interface{}) {
	klog.Fatalf(format, args...)
}
