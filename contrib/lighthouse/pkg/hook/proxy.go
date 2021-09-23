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

package hook

import (
	"net/http"

	"github.com/docker/go-connections/sockets"
	"k8s.io/klog"

	"github.com/tencent/lighthouse/pkg/httputil"
	"github.com/tencent/lighthouse/pkg/util"
)

type reverseProxy struct {
	proxy *httputil.ReverseProxy
}

func newReverseProxy(remoteEndpoint string) *reverseProxy {
	proto, addr, err := util.GetProtoAndAddress(remoteEndpoint)
	if err != nil {
		klog.Fatalf("can't parse remote endpoint %s, %v", remoteEndpoint, err)
	}

	tr := new(http.Transport)
	sockets.ConfigureTransport(tr, proto, addr)

	rp := &reverseProxy{
		proxy: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = addr
				if _, ok := req.Header["User-Agent"]; !ok {
					// explicitly disable User-Agent so it's not set to default value
					req.Header.Set("User-Agent", "")
				}
			},
			Transport: tr,
		},
	}

	return rp
}

// ServeHTTP remote request handle interface
func (rp *reverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rp.proxy.ServeHTTP(w, req)
}
