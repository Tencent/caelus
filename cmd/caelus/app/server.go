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
	"os"
	"strings"

	"github.com/tencent/caelus/cmd/caelus/context"
	"github.com/tencent/caelus/pkg/caelus/alarm"
	"github.com/tencent/caelus/pkg/caelus/checkpoint"
	"github.com/tencent/caelus/pkg/caelus/cpi"
	"github.com/tencent/caelus/pkg/caelus/diskquota"
	"github.com/tencent/caelus/pkg/caelus/healthcheck"
	"github.com/tencent/caelus/pkg/caelus/healthcheck/conflict"
	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/online"
	"github.com/tencent/caelus/pkg/caelus/predict"
	"github.com/tencent/caelus/pkg/caelus/qos"
	"github.com/tencent/caelus/pkg/caelus/resource"
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"
	common "github.com/tencent/caelus/pkg/util"
	"github.com/tencent/caelus/pkg/version/verflag"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/tools/cache"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"
)

// options show the supported flags
type options struct {
	Master           string
	Kubeconfig       string
	FeatureGates     map[string]bool
	HostnameOverride string
	Config           string

	// flags related to server
	ApiOption ApiOption
}

// this describe the common module functions
type module interface {
	// Run describe how the module works, this should be started asynchronously
	Run(stop <-chan struct{})
	// Name return module name
	Name() string
}

// printFlags show all flags
func printFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})
}

// NewServerCommand initialize server execution context
func NewServerCommand() *cobra.Command {
	opts := newOptions()

	cmd := &cobra.Command{
		Use: "caelus",
		Run: func(cmd *cobra.Command, args []string) {
			verflag.PrintAndExitIfRequested()
			printFlags(cmd.Flags())

			if err := opts.Complete(); err != nil {
				klog.Exitf("can't complete command, %v", err)
			}

			if err := opts.Run(); err != nil {
				klog.Exitf("can't run command, %v", err)
			}
		},
	}

	opts.AddFlags(cmd.Flags())

	return cmd
}

// newOptions return the options instance
func newOptions() *options {
	return &options{}
}

// AddFlags describe server flags
func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig,
		"The path of kubernetes config to communicate with Kubernetes")
	fs.StringVar(&o.HostnameOverride, "hostname-override", o.HostnameOverride,
		"If non-empty, will use this string as identification instead of the actual hostname.")
	fs.StringVar(&o.Master, "master", o.Master, "apiserver master address")
	fs.Var(cliflag.NewMapStringBool(&o.FeatureGates), "feature-gates",
		"A set of key=value pairs that describe feature gates for alpha/experimental features. Options are:\n"+
			strings.Join(utilfeature.DefaultMutableFeatureGate.KnownFeatures(), "\n"))
	fs.StringVar(&o.Config, "config", "/etc/sysconfig/caelus.json", "The Config file")

	// flags related to API
	o.ApiOption.AddFlags(fs)
}

// Complete describe feature gate flags
func (o *options) Complete() error {
	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(o.FeatureGates); err != nil {
		return err
	}
	return nil
}

// Run starts the main loop
func (o *options) Run() error {
	// get node name, which using in kubernetes
	nodeName := o.HostnameOverride
	if len(nodeName) == 0 {
		hostName, err := os.Hostname()
		if err != nil {
			return err
		}
		nodeName = hostName
	}
	util.SetNodeName(nodeName)

	// get node ip
	nodeIP := o.ApiOption.InsecureAddress
	if util.MatchIP(nodeName) {
		nodeIP = nodeName
	}
	util.SetNodeIP(nodeIP)

	// parse config file
	caelus, err := types.ParseJsonConfig(o.Config)
	if err != nil {
		return err
	}

	signalCh := common.SetupSignalHandler()
	ctx := &context.CaelusContext{
		Master:     o.Master,
		Kubeconfig: o.Kubeconfig,
		NodeName:   nodeName,
	}

	// initialize API related context
	if err := o.ApiOption.Init(ctx); err != nil {
		return err
	}

	// the ctx module would start informers, so must be added firstly
	modules := append([]module{ctx}, o.initModules(caelus, ctx)...)
	// start all modules
	for _, module := range modules {
		klog.V(2).Infof("%s starting", module.Name())
		module.Run(signalCh)
		klog.V(2).Infof("%s started", module.Name())
	}
	// start http server
	if err := o.ApiOption.RegisterServer(nodeIP); err != nil {
		return err
	}
	if err := o.ApiOption.Run(signalCh); err != nil {
		return err
	}

	klog.Infof("caleus starting success")
	<-signalCh
	return nil
}

// initModules initialize all manager client
func (o *options) initModules(caelus *types.CaelusConfig, ctx *context.CaelusContext) []module {
	var modules []module
	var stStore statestore.StateStore
	var predictor predict.Interface
	var resourceManager resource.Interface
	var qosManager qos.Manager
	var alarmManager *alarm.Manager
	var onlineManager online.Interface
	var diskquotaManager diskquota.DiskQuotaInterface
	var healthCheckManager health.Manager
	var conflictMn conflict.Manager
	podInformer := ctx.GetPodFactory().Core().V1().Pods().Informer()

	// resource store
	stStore = statestore.NewStateStoreManager(&caelus.Metrics, &caelus.Online, podInformer)
	modules = append(modules, stStore)
	// register prometheus metrics
	o.ApiOption.loadStatsMetric(metrics.StatsMetricStore, stStore)
	// conflict manager
	conflictMn = conflict.NewConflictManager()
	// predict manager
	predictor = predict.NewPredictorOrDie(caelus.Predicts, stStore)
	modules = append(modules, predictor)
	// offline resource manager
	if !caelus.NodeResource.Disable {
		resourceData := generateResourceManagerData(caelus, stStore, podInformer, ctx)
		resourceManager = resource.NewOfflineOnK8sManager(caelus.NodeResource, predictor, conflictMn, resourceData)
		o.ApiOption.prometheusRegistry.MustRegister(resourceManager)
		modules = append(modules, resourceManager)
	}
	// alarm manager
	alarmManager = alarm.NewManager(&caelus.Alarm, resourceManager)
	modules = append(modules, alarmManager)
	// online jobs manager
	if caelus.Online.Enable {
		onlineManager = online.NewOnlineManager(caelus.Online, stStore)
		modules = append(modules, onlineManager)
	}
	// resource isolation manager
	if !caelus.ResourceIsolate.Disable {
		qosManager = qos.NewQosK8sManager(caelus.ResourceIsolate, stStore, predictor,
			podInformer, conflictMn, onlineManager)
		modules = append(modules, qosManager)
	}
	// cpi manager
	if caelus.CpiManager.Enable {
		if err := cpi.InitManager(caelus.CpiManager); err != nil {
			klog.Fatalf("init cpi manager failed: %v", err)
		}
	}
	// disk quota manager
	if caelus.DiskQuota.Enabled {
		diskquotaManager = diskquota.NewDiskQuota(caelus.DiskQuota, caelus.K8sConfig, podInformer)
		modules = append(modules, diskquotaManager)
		o.ApiOption.loadStatsMetric(metrics.StatsMetricDiskQuota, diskquotaManager)
	}
	// health check manager
	healthCheckManager = health.NewHealthManager(types.InitHealthCheckConfigFunc(&caelus.Metrics.Node,
		&caelus.Predicts[0].ReserveResource), stStore, resourceManager, qosManager,
		conflictMn, podInformer)
	modules = append(modules, healthCheckManager)

	return modules
}

// generateResourceManagerData construct offline resource struct data
func generateResourceManagerData(caelus *types.CaelusConfig, stStore statestore.StateStore,
	podInformer cache.SharedIndexInformer, ctx *context.CaelusContext) interface{} {
	checkpointCfg := caelus.CheckPoint
	nodeInformer := ctx.GetNodeFactory().Core().V1().Nodes().Informer()

	rsCheckpointManager, err := checkpoint.NewNodeResourceCheckpointManager(checkpointCfg.CheckPointDir,
		checkpointCfg.NodeResourceKey)
	if err != nil {
		klog.Fatalf("init checkpoint fail: %v", err)
	}

	var resourceData interface{}
	offlineOnK8sCommonData := resource.OfflineOnK8sCommonData{
		StStore:           stStore,
		Client:            ctx.GetKubeClient(),
		PodInformer:       podInformer,
		CheckpointManager: rsCheckpointManager,
	}
	if types.OfflineOnYarn(&caelus.TaskType) {
		resourceData = &resource.OfflineYarnData{
			OfflineOnK8sCommonData: offlineOnK8sCommonData,
		}
	} else {
		resourceData = &resource.OfflineK8sData{
			OfflineOnK8sCommonData: offlineOnK8sCommonData,
			NodeInformer:           nodeInformer,
		}
	}

	return resourceData
}
