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

package ports

import (
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

// Unused check if a host port is available
func Unused(hp *Hostport) bool {
	closer, err := openLocalPort(hp)
	if err != nil {
		return false
	}
	if err := closer.Close(); err != nil {
		klog.Warningf("can't close port %v", hp)
	}
	return true
}

// FindUnusedPort find a host port that is available
func FindUnusedPort(start int, blackList sets.Int, protocol string) (int, error) {
	hp := &Hostport{
		Port:     0,
		Protocol: protocol,
	}
	for i := start; i < 30000; i++ {
		if blackList.Has(i) {
			continue
		}
		hp.Port = i
		if Unused(hp) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("can't find an unused port")
}

// closeable is a closable resource
type closeable interface {
	Close() error
}

// Hostport is a host Port
type Hostport struct {
	Port     int
	Protocol string
}

// String print the host port
func (hp *Hostport) String() string {
	return fmt.Sprintf("%s:%d", hp.Protocol, hp.Port)
}

// this function is copied from github.com/kubernetes/kubernetes/pkg/kubelet/network/kubenet/kubenet_linux.go
func openLocalPort(hp *Hostport) (closeable, error) {
	var socket closeable
	switch hp.Protocol {
	case "tcp":
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", hp.Port))
		if err != nil {
			return nil, err
		}
		socket = listener
		hp.Port = listener.Addr().(*net.TCPAddr).Port
	case "udp":
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", hp.Port))
		if err != nil {
			return nil, err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return nil, err
		}
		socket = conn
		hp.Port = conn.LocalAddr().(*net.UDPAddr).Port
	default:
		return nil, fmt.Errorf("unknown Protocol %q", hp.Protocol)
	}
	klog.V(4).Infof("Opened local Port %s", hp.String())
	return socket, nil
}
