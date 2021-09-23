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

package test

import (
	"fmt"
	"net"
	"net/http"

	"github.com/google/uuid"
)

// UnixSocketServer mock socket server
type UnixSocketServer struct {
	addr string
	mux  *http.ServeMux
	l    net.Listener
}

// NewUnixSocketServer new mock socket server instance
func NewUnixSocketServer() *UnixSocketServer {
	return &UnixSocketServer{
		addr: fmt.Sprintf("@%s", uuid.New().String()),
		mux:  http.NewServeMux(),
	}
}

// GetAddress return socket address
func (uss *UnixSocketServer) GetAddress() string {
	return fmt.Sprintf("unix://%s", uss.addr)
}

// RegisterHandler register to mock server
func (uss *UnixSocketServer) RegisterHandler(path string, handler http.HandlerFunc) {
	uss.mux.HandleFunc(path, handler)
}

// Start main loop
func (uss *UnixSocketServer) Start() error {
	l, err := net.Listen("unix", uss.addr)
	if err != nil {
		return err
	}

	uss.l = l

	return http.Serve(l, uss.mux)
}

// Stop stop mock socket server
func (uss *UnixSocketServer) Stop() error {
	if uss.l != nil {
		return uss.l.Close()
	}
	return nil
}
