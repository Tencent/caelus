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

package app

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/tencent/caelus/pkg/nm-operator/nmoperator"
	"github.com/tencent/caelus/pkg/nm-operator/types"
	"github.com/tencent/caelus/pkg/version/verflag"

	"github.com/emicklei/go-restful"
	"github.com/spf13/pflag"
)

// ApiOption describe API related data
type ApiOption struct {
	Profiling       bool
	InsecureAddress string
	InsecurePort    string
	EnableCadvisor  bool
}

// AddFlags describe API related flags
func (a *ApiOption) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&a.Profiling, "profiling", true,
		"Enable profiling via web interface host:port/debug/pprof/. Default is true")
	fs.StringVar(&a.InsecureAddress, "insecure-bind-address", "127.0.0.1",
		"The IP address on which to serve the --insecure-port. Defaults to localhost")
	fs.StringVar(&a.InsecurePort, "insecure-port", "10003",
		"The port on which to serve unsecured, unauthenticated access. Default 10003")
	fs.BoolVar(&a.EnableCadvisor, "enable-cadvisor", false,
		"enable cadvisor to collect container resources")
}

// Init function do some initialization operations
func (a *ApiOption) Init() error {
	// nothing to do for now
	return nil
}

// RegisterServer register API route and start listening
func (a *ApiOption) RegisterServerAndListen() error {
	mux := http.NewServeMux()
	if a.Profiling {
		// pporf debug support
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	}

	// version support
	mux.HandleFunc("/version", verflag.RequestVersion)

	// nodemanager operation support
	container := restful.NewContainer()
	container.ServeMux = mux
	container.Router(restful.CurlyRouter{})
	cors := restful.CrossOriginResourceSharing{
		AllowedHeaders: []string{"Content-Type", "Accept", "Origin"},
		// now just support GET and POST, users could add other methods if needed in future
		AllowedMethods: []string{"GET", "POST"},
		CookiesAllowed: false,
		Container:      container,
	}
	container.Filter(cors.Filter)
	container.Filter(container.OPTIONSFilter)
	// register nodemanager operation route
	nmoperator.RegisterNMOperatorService(a.EnableCadvisor, container)

	handler := http.TimeoutHandler(mux, time.Duration(1*time.Minute), "time out")
	httpServer := &http.Server{
		Handler:        handler,
		MaxHeaderBytes: 1 << 20,
	}

	// get port from environment
	portStr := os.Getenv(types.EnvPort)
	if portStr != "" {
		a.InsecurePort = portStr
	}

	insecureLocation := net.JoinHostPort(a.InsecureAddress, a.InsecurePort)
	listener, err := net.Listen("tcp", insecureLocation)
	if err != nil {
		return fmt.Errorf("listen(%s) err: %v", insecureLocation, err)
	}

	return httpServer.Serve(listener)
}
