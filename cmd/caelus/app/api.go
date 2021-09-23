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
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/tencent/caelus/cmd/caelus/context"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/metrics/outer/serverrequest"
	"github.com/tencent/caelus/pkg/caelus/metrics/outer/textfile"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/version/verflag"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	"k8s.io/klog"
)

// ApiOption describe API related data
type ApiOption struct {
	// Profiling enable pprof debug
	Profiling       bool
	InsecureAddress string
	InsecurePort    string
	// statsMetric describe metrics which need to show in prometheus mode
	statsMetric map[metrics.StatsMetric]interface{}
	// prometheusRegistry used for register prometheus metrics
	prometheusRegistry *prometheus.Registry
}

// AddFlags describe API related flags
func (a *ApiOption) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&a.Profiling, "profiling", true,
		"Enable profiling via web interface host:port/debug/pprof/. Default is true")
	fs.StringVar(&a.InsecureAddress, "insecure-bind-address", "127.0.0.1",
		"The IP address on which to serve the --insecure-port. Defaults to localhost")
	fs.StringVar(&a.InsecurePort, "insecure-port", "10030",
		"The port on which to serve unsecured, unauthenticated access. Default 10030")
}

// Init function init metrics collecting manager
func (a *ApiOption) Init(ctx *context.CaelusContext) error {
	a.statsMetric = make(map[metrics.StatsMetric]interface{})
	a.prometheusRegistry = prometheus.NewRegistry()
	return nil
}

// Run main loop, now nothing to do
func (a *ApiOption) Run(stopCh <-chan struct{}) error {
	return nil
}

func (a *ApiOption) loadStatsMetric(name metrics.StatsMetric, stat interface{}) {
	a.statsMetric[name] = stat
}

// RegisterServer register API route
func (a *ApiOption) RegisterServer(node string) error {
	// start server
	mux := http.NewServeMux()
	if a.Profiling {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	}

	// register prometheus metrics
	metrics.RegisterTotalMetrics(a.prometheusRegistry)
	textfile.RegisterTextFileMetrics(a.prometheusRegistry)
	stStore := a.statsMetric[metrics.StatsMetricStore].(statestore.StateStore)
	serverrequest.RegisterRequestMetrics(a.prometheusRegistry, stStore)
	a.prometheusRegistry.MustRegister(metrics.NewPrometheusCollector(node, a.statsMetric,
		metrics.ResourceMetricsConfig()))
	mux.Handle("/metrics", promhttp.HandlerFor(a.prometheusRegistry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError}))

	// register version api
	mux.HandleFunc("/version", verflag.RequestVersion)

	mux.HandleFunc("/klog", func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query().Get("v")
		if v != "" {
			flag.Lookup("v").Value.Set(v)
			klog.Infof("set log level to %v", v)
		}
	})

	handler := http.TimeoutHandler(mux, time.Duration(1*time.Minute), "time out")
	httpServer := &http.Server{
		Handler:        handler,
		MaxHeaderBytes: 1 << 20,
	}

	insecureLocation := net.JoinHostPort(a.InsecureAddress, a.InsecurePort)
	listener, err := net.Listen("tcp", insecureLocation)
	if err != nil {
		return fmt.Errorf("listen(%s) err: %v", insecureLocation, err)
	}

	go func() {
		err = httpServer.Serve(listener)
		if err != nil {
			err = fmt.Errorf("start listen server err: %v", err)
		}
	}()

	return err
}
