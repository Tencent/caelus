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

package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"

	systemd "github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"github.com/tencent/lighthouse-plugin/pkg/plugin"
)

// socket protocol
const UnixProto = "unix"

type pluginServer struct {
	listenAddress     string
	mux               *mux.Router
	client            kubernetes.Interface
	dockerEndpoint    string
	dockerVersion     string
	ignoredNamespaces []string
}

// NewPluginServer new the plugin server instance
func NewPluginServer(
	addr string,
	client kubernetes.Interface,
	dockerEndpoint,
	dockerVersion string,
	ignoredNamespaces []string) *pluginServer {
	ps := &pluginServer{
		listenAddress:     addr,
		mux:               mux.NewRouter(),
		client:            client,
		dockerEndpoint:    dockerEndpoint,
		dockerVersion:     dockerVersion,
		ignoredNamespaces: ignoredNamespaces,
	}
	// register plugins
	ps.registerPlugin()
	return ps
}

// registerPlugin register all supported plugins
func (ps *pluginServer) registerPlugin() {
	filter := sets.NewString(ps.ignoredNamespaces...)
	filterNamespaceFunc := func(ns string) bool {
		return filter.Has(ns)
	}

	// plugins are ready by calling "init" function
	for name, p := range plugin.AllPlugins {
		klog.Infof("Register plugin %s, method %s, path %s", name, p.Method(), p.Path())
		p.SetIgnored(filterNamespaceFunc)

		ps.mux.Methods(p.Method()).Path(
			p.Path()).Handler(p.Handler(ps.client, ps.dockerEndpoint, ps.dockerVersion))
	}
}

// Run main loop
func (ps *pluginServer) Run(stop <-chan struct{}) error {
	proto, addr, err := getProtoAndAddress(ps.listenAddress)
	if err != nil {
		return err
	}

	// start listening
	l, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}

	ready := make(chan struct{})
	ch := make(chan error)
	go func() {
		close(ready)
		ch <- http.Serve(l, ps)
	}()

	<-ready
	klog.Infof("Hook manager is running")

	sent, err := systemd.SdNotify(true, "READY=1\n")
	if err != nil {
		klog.Warningf("Unable to send systemd daemon successful start message: %v\n", err)
	}

	if !sent {
		klog.Warningf("Unable to send systemd daemon Type=notify in systemd service file?")
	}

	select {
	case <-stop:
	case e := <-ch:
		return e
	}

	return nil
}

// ServeHTTP register http request route
func (ps *pluginServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var match mux.RouteMatch
	if ps.mux.Match(req, &match) {
		klog.V(4).Infof("Handle request %s %s", req.Method, req.URL.Path)
		ps.mux.ServeHTTP(w, req)
		return
	}
	klog.V(4).Infof("Unhandled request %s %s", req.Method, req.URL.Path)
}

// getProtoAndAddress parse "endpoint" into protocol and address
func getProtoAndAddress(endpoint string) (proto string, addr string, err error) {
	seps := strings.SplitN(endpoint, "://", 2)
	if len(seps) != 2 {
		err = fmt.Errorf("malformed unix socket")
		return
	}
	if len(seps[1]) > len(syscall.RawSockaddrUnix{}.Path) {
		err = fmt.Errorf("unix socket path %q is too long", seps[1])
		return
	}

	proto = seps[0]
	addr = seps[1]
	// check if the socket protocol is supported
	if proto != UnixProto {
		err = fmt.Errorf("only support Unix protocol")
		return
	}

	if !strings.HasPrefix(addr, "@") {
		err = os.Remove(addr)
		if err != nil && !os.IsNotExist(err) {
			return
		}
		err = nil
	}
	return proto, addr, err
}
