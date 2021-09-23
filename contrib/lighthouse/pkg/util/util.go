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

package util

import (
	"fmt"
	"net/http"
	"strings"
	"syscall"

	"github.com/docker/go-connections/sockets"
	"github.com/spf13/pflag"
	"k8s.io/klog"
)

const (
	// socket protocol
	UnixProto = "unix"
)

// BuildClientOrDie build http client
func BuildClientOrDie(endpoint string) *http.Client {
	proto, addr, err := GetProtoAndAddress(endpoint)
	if err != nil {
		klog.Fatalf("can't parse endpoint %s, %v", endpoint, err)
	}

	if proto != UnixProto {
		klog.Fatalf("only support unix socket")
	}

	tr := new(http.Transport)
	sockets.ConfigureTransport(tr, proto, addr)
	return &http.Client{
		Transport: tr,
	}
}

// GetProtoAndAddress get protocol and address from url
func GetProtoAndAddress(endpoint string) (string, string, error) {
	seps := strings.SplitN(endpoint, "://", 2)
	if len(seps) != 2 {
		return "", "", fmt.Errorf("malformed unix socket")
	}

	if len(seps[1]) > len(syscall.RawSockaddrUnix{}.Path) {
		return "", "", fmt.Errorf("unix socket path %q is too long", seps[1])
	}

	return seps[0], seps[1], nil
}

// PrintFlags print flags
func PrintFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})
}
