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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	systemd "github.com/coreos/go-systemd/v22/daemon"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	"github.com/tencent/lighthouse/pkg/apis/componentconfig.lighthouse.io/v1alpha1"
	"github.com/tencent/lighthouse/pkg/util"
)

type hookManager struct {
	timeout time.Duration
	// read from config file
	listenAddress string
	mux           *mux.Router
	// docker server
	backend http.Handler
}

// hookHandleKey group http request options
type hookHandleKey struct {
	Method     string
	URLPattern string
}

// hookHandleData group kinds of hook handler
type hookHandleData struct {
	preHooks  []HookHandler
	postHooks []HookHandler
}

// NewHookManager return a hook manager to process hook requests
func NewHookManager() *hookManager {
	hm := &hookManager{
		mux: mux.NewRouter(),
	}

	return hm
}

// Run start the hook manager
func (hm *hookManager) Run(stop <-chan struct{}) error {
	proto, addr, err := util.GetProtoAndAddress(hm.listenAddress)
	if err != nil {
		return err
	}

	/** Abstract unix socket is not supported */
	if proto == util.UnixProto {
		if strings.HasPrefix(addr, "@") {
			klog.Fatalf("can't use abstract unix socket %s", addr)
		}
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	l, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}

	ready := make(chan struct{})
	ch := make(chan error)
	go func() {
		close(ready)
		ch <- http.Serve(l, hm)
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

	// waiting stop or error signal
	select {
	case <-stop:
	case e := <-ch:
		return e
	}

	return nil
}

// InitFromConfig initialize the hook manager with specific configuration
func (hm *hookManager) InitFromConfig(config *v1alpha1.HookConfiguration) error {
	klog.Infof("Hook timeout: %d seconds", config.Timeout)
	hm.timeout = config.Timeout * time.Second
	// init docker daemon server
	hm.backend = newReverseProxy(config.RemoteEndpoint)
	hm.listenAddress = config.ListenAddress

	hooksMap := make(map[hookHandleKey]*hookHandleData)
	for _, r := range config.WebHooks {
		klog.Infof("Register hook %s, endpoint %s", r.Name, r.Endpoint)
		// register hook by creating the connector
		hc := newHookConnector(r.Name, r.Endpoint, r.FailurePolicy)
		for _, fp := range r.Stages {
			klog.Infof("Register %s %s %s with %s", fp.Type, fp.Method, fp.URLPattern, hc.endpoint)
			key := hookHandleKey{
				Method:     fp.Method,
				URLPattern: fp.URLPattern,
			}

			hookData, found := hooksMap[key]
			if !found {
				// must initialize the hook handler data
				hookData = &hookHandleData{
					preHooks:  make([]HookHandler, 0),
					postHooks: make([]HookHandler, 0),
				}
			}

			switch fp.Type {
			case v1alpha1.PreHookType:
				hookData.preHooks = append(hookData.preHooks, hc)
				hooksMap[key] = hookData
			case v1alpha1.PostHookType:
				hookData.postHooks = append(hookData.postHooks, hc)
				hooksMap[key] = hookData
			}
		}
	}

	// build router for different request method and URL pattern
	for k, v := range hooksMap {
		klog.V(2).Infof("Build router: %s %s", k.Method, k.URLPattern)
		preHookChainHandler := hm.buildPreHookHandlerFunc(v.preHooks)
		postHookChainHandler := hm.buildPostHookHandlerFunc(v.postHooks)

		// construct http request handler
		hm.mux.Methods(k.Method).Path(k.URLPattern).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := preHookChainHandler(w, r); err != nil {
				return
			}

			klog.V(4).Infof("Send data to backend path %s", r.URL.Path)
			recorder := httptest.NewRecorder()
			hm.backend.ServeHTTP(recorder, r)
			klog.V(4).Infof("Finish backend path %s", r.URL.Path)

			postHookChainHandler(recorder, r)
			for k, vs := range recorder.Header() {
				for _, v := range vs {
					w.Header().Set(k, v)
				}
			}
			w.WriteHeader(recorder.Code)
			w.Write(recorder.Body.Bytes())
		})
	}

	return nil
}

// applyHook transfer hook request to backend server
func (hm *hookManager) applyHook(ctx context.Context, handlers []HookHandler, hookType v1alpha1.HookType,
	method, path string, body *[]byte) error {
	var err error

	for idx, h := range handlers {
		hookErr := func() error {
			klog.V(4).Infof("Send to %s handler %d", hookType, idx)
			patch := &PatchData{}

			// do hook request to backend server
			switch hookType {
			case v1alpha1.PreHookType:
				if err := h.PreHook(ctx, patch, method, path, *body); err != nil {
					klog.Errorf("preHook failed, %v", err)
					return err
				}
			case v1alpha1.PostHookType:
				if err := h.PostHook(ctx, patch, method, path, *body); err != nil {
					klog.Errorf("postHook failed, %v", err)
					return err
				}
			}

			if patch.PatchData == nil {
				return nil
			}

			// handle different patch type
			switch types.PatchType(patch.PatchType) {
			case types.JSONPatchType:
				p, err := jsonpatch.DecodePatch(patch.PatchData)
				if err != nil {
					klog.Errorf("can't decode patch, %v", err)
					return err
				}
				*body, err = p.Apply(*body)
				if err != nil {
					klog.Errorf("can't apply patch, %v", err)
					return err
				}
			case types.MergePatchType:
				*body, err = jsonpatch.MergePatch(*body, patch.PatchData)
				if err != nil {
					klog.Errorf("can't merge patch, %v", err)
					return err
				}
			default:
				return fmt.Errorf("unknown patch type: %s", patch.PatchType)
			}

			return nil
		}()

		if hookErr == nil {
			continue
		}

		klog.Errorf("can't perform %s %s %s, %v", hookType, method, path, hookErr)
		return hookErr
	}

	return nil
}

// buildPostHookHandlerFunc build post hook handler function
func (hm *hookManager) buildPostHookHandlerFunc(handlers []HookHandler) PostHookFunc {
	return func(w *httptest.ResponseRecorder, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), hm.timeout)
		defer cancel()

		data := &PostHookData{
			StatusCode: w.Code,
			Body:       w.Body.Bytes(),
		}

		w.Body.Reset()

		bodyBytes, err := json.Marshal(data)
		if err != nil {
			klog.Errorf("can't marshal post hook data, %v", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		// do hook request
		klog.V(4).Infof("PostHook request %s, body: %s", r.URL.Path, string(bodyBytes))
		if err := hm.applyHook(ctx, handlers, v1alpha1.PostHookType, r.Method, r.URL.Path, &bodyBytes); err != nil {
			klog.Errorf("can't perform postHook, %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		bodyBytes = fixUnexpectedEscape(bodyBytes)
		if err := json.Unmarshal(bodyBytes, data); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write(data.Body)
	}
}

// buildPreHookHandlerFunc build pre hook handler function
func (hm *hookManager) buildPreHookHandlerFunc(handlers []HookHandler) PreHookFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		ctx, cancel := context.WithTimeout(context.Background(), hm.timeout)
		defer cancel()

		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			klog.Errorf("can't read request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return err
		}

		// do hook request
		klog.V(4).Infof("PreHook request %s, body: %s", r.URL.Path, string(bodyBytes))
		if err := hm.applyHook(ctx, handlers, v1alpha1.PreHookType, r.Method, r.URL.Path, &bodyBytes); err != nil {
			klog.Errorf("can't perform preHook, %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return err
		}

		bodyBytes = fixUnexpectedEscape(bodyBytes)
		newBody := bytes.NewBuffer(bodyBytes)
		r.Body = ioutil.NopCloser(newBody)
		r.ContentLength = int64(newBody.Len())
		return nil
	}
}

// ServeHTTP implement http.Handler
func (hm *hookManager) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var match mux.RouteMatch
	if hm.mux.Match(req, &match) {
		klog.V(5).Infof("Handle request %s %s", req.Method, req.URL.Path)
		hm.mux.ServeHTTP(w, req)
		return
	}

	if strings.HasPrefix(req.URL.Path, "/debug/pprof") {
		pprof.Index(w, req)
		return
	}

	klog.V(5).Infof("Unhandled request %s %s", req.Method, req.URL.Path)
	hm.backend.ServeHTTP(w, req)
}
