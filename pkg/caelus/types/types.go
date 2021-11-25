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

package types

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
	"github.com/tencent/caelus/pkg/util/times"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

// MetricKind represent the kind of metrics that cAdvisor exposes.
type MetricKind string

const (
	// TaskType
	OnlineTypeOnK8s      = "k8s"
	OnlineTypeOnLocal    = "local"
	OfflineTypeOnk8s     = "k8s"
	OfflineTypeYarnOnk8s = "yarn_on_k8s"

	defaultYarnScheduleServerPort    = "10010"
	defaultCheckPointDir             = "/tmp/caeclus"
	defaultNodeResourceCheckPointKey = "node_resource"
	defaultDisksToCore               = 10
	// defaultMinDiskCapacity is nm local-dirs min capacity(unit: Gb).
	defaultMinDiskCapacity = 100

	AlarmTypeLocal  = "local"
	AlarmTypeRemote = "remote"

	// CpuManagePolicyBT is just for tencent OS
	CpuManagePolicyBT       = "bt"
	CpuManagePolicySet      = "cpuset"
	CpuManagePolicyQuota    = "quota"
	CpuManagePolicyAdaptive = "adaptive"

	// MemUnit translates Mb to byte
	MemUnit   = int64(1024 * 1024)
	MemGbUnit = int64(1024 * 1024 * 1024)
	// CpuUnit translates milli core
	CpuUnit = int64(1000)
	// DiskUnit translates Gi to btye
	DiskUnit = int64(1024 * 1020 * 1024)

	// pod annotation fixed annotation
	PodAnnotationPrefix = "mixer.kubernetes.io/"

	// RootFS is the root directory in container.
	RootFS                     = "/rootfs"
	CgroupKubePods             = "/kubepods"
	CgroupOffline              = "/kubepods/offline"
	CgroupOfflineSystem        = CgroupOffline + "/system"
	SystemComponentOomScoreAdj = "500"
	CgroupYarn                 = "hadoop-yarn"
	// CgroupNonK8sOnline is the cgroup for online jobs, which are not running on k8s, we need to create the cgroup
	// and children cgroup manually.
	CgroupNonK8sOnline = "/onlinejobs"

	// default config
	defaultCheckPeriod             = times.Duration(5 * time.Minute)
	defaultMetricsInterval         = times.Duration(50 * time.Second)
	defaultContainerCgroups        = "/kubepods"
	defaultPerfDuration            = times.Duration(10 * time.Second)
	defaultRdtDuration             = times.Duration(1 * time.Second)
	defaultRdtCommand              = "/usr/bin/pqos"
	defaultRdtInterval             = times.Duration(1 * time.Second)
	defaultResourceIsolateInterval = times.Duration(10 * time.Minute)
	defaultNodeUpdateInterval      = times.Duration(10 * time.Second)
	defaultAlarmMessageBatch       = 10
	defaultAlarmMessageDelay       = times.Duration(1 * time.Minute)
	defaultCPIWindowDuration       = times.Duration(10 * time.Minute)
	defaultCPIMaxJobSpecRange      = times.Duration(24 * time.Hour)
	defaultDiskSpaceCheckPeriod    = times.Duration(30 * time.Minute)
	defaultOnlineCheckInterval     = times.Duration(10 * time.Minute)
	defaultOnlineBatchNumber       = 10
	defaultMetricsRequestInterval  = times.Duration(1 * time.Minute)

	defaultCpuOverCommitPercent = 1.2
)

var (
	// AvailableOnlineTaskType describe available online tasks, which may be pod or local process
	AvailableOnlineTaskType = sets.NewString(OnlineTypeOnK8s, OnlineTypeOnLocal)
	// AvailableOfflineTaskType describe available offline tasks, which may be pod or yarn job
	AvailableOfflineTaskType = sets.NewString(OfflineTypeOnk8s, OfflineTypeYarnOnk8s)

	// AvailableAlarmType shows available alarm type
	AvailableAlarmType = sets.NewString(AlarmTypeLocal, AlarmTypeRemote)

	// AvailableCpuManagePolicy shows available cpu manage policy
	AvailableCpuManagePolicy = sets.NewString(CpuManagePolicyBT, CpuManagePolicySet, CpuManagePolicyQuota,
		CpuManagePolicyAdaptive)

	CompressibleRes = sets.NewString(string(v1.ResourceCPU))
)

// OfflineJobs describe offline job features, such as resource and state
type OfflineJobs struct {
	Metadata interface{}
	Request  v1.ResourceList
	Used     v1.ResourceList
	State    string
}

// ResourceUpdateEvent define the event when need to update offline resources
type ResourceUpdateEvent struct {
	ConflictRes []string
	Reason      string
}

// Resource is the cpu and memory configuration
type Resource struct {
	CpuMilli      *float64 `json:"cpu_milli"`
	MemMB         *float64 `json:"mem_mb"`
	CpuPercentStr string   `json:"cpu_percent"`
	CpuPercent    *float64 `json:"-"`
	MemPercentStr string   `json:"mem_percent"`
	MemPercent    *float64 `json:"-"`
}

// DiskPartitionStats show disk space size
type DiskPartitionStats struct {
	// TotalSize show total disk size in bytes
	TotalSize int64
	// UsedSize show used disk size in bytes
	UsedSize int64
	// FreeSize show free disk size in bytes
	FreeSize int64
}

// CaelusConfig is the configuration for Caelus
type CaelusConfig struct {
	K8sConfig    K8sConfig          `json:"k8s_config"`
	CheckPoint   CheckPointConfig   `json:"check_point"`
	TaskType     TaskTypeConfig     `json:"task_type"`
	NodeResource NodeResourceConfig `json:"node_resource"`
	// If multiple predicts, the first one is used for real prediction. The left are experiment predicts, caelus will
	// only feeds samples to them and expose predict metrics for them.
	Predicts        []PredictConfig       `json:"predicts"`
	Metrics         MetricsCollectConfig  `json:"metrics"`
	ResourceIsolate ResourceIsolateConfig `json:"resource_isolate"`
	CpiManager      CPIManagerConfig      `json:"cpi_manager"`
	Alarm           AlarmConfig           `json:"alarm"`
	Online          OnlineConfig          `json:"online"`
	DiskQuota       DiskQuotaConfig       `json:"disk_quota"`
}

// K8sConfig show kubernetes config
type K8sConfig struct {
	KubeletRootDir string `json:"kubelet_root_dir"`
}

// CheckPointConfig group info related to check point, which saving state to local file
type CheckPointConfig struct {
	CheckPointDir   string `json:"check_point_dir"`
	NodeResourceKey string `json:"node_resource_key"`
}

// TaskTypeConfig show the online and offline task type,
// such as offline is yarn on k8s.
type TaskTypeConfig struct {
	OnlineType  string `json:"online_type"`
	OfflineType string `json:"offline_type"`
}

// ComponentConfig is the config to specific a non-containerized component
type ComponentConfig struct {
	Cgroup  string `json:"cgroup"`
	Command string `json:"command"`
}

// NodeResourceConfig group configuration for node
type NodeResourceConfig struct {
	Disable        bool           `json:"disable"`
	UpdateInterval times.Duration `json:"update_interval"`
	OfflineType    string         `json:"-"`
	// DisableKillIfNormal does not kill pod when no resource in conflicting status
	DisableKillIfNormal         bool                   `json:"disable_kill_if_normal"`
	OnlyKillIfIncompressibleRes bool                   `json:"only_kill_if_incompressible_res"`
	YarnConfig                  YarnNodeResourceConfig `json:"yarn_config"`
	Silence                     SilenceConfig          `json:"silence"`
}

// SilenceConfig describe the period time, do not allow running offline jobs
type SilenceConfig struct {
	// [0:00:00, 5:00:00]
	Periods [][2]times.SecondsInDay `json:"periods"`
	// disable schedule before silence
	AheadOfUnSchedule times.Duration `json:"ahead_of_unSchedule"`
}

// YarnNodeResourceConfig is used to show yarn related configuration
type YarnNodeResourceConfig struct {
	// CapacityIncInterval is used to make nodemanager capacity increase not very frequently
	CapacityIncInterval times.Duration    `json:"capacity_inc_interval"`
	NMServer            string            `json:"nm_server"`
	NMReserve           Resource          `json:"nm_reserve"`
	ResourceRoundOff    RoundOffResource  `json:"resource_roundoff"`
	ResourceRange       RangeResource     `json:"resource_range"`
	ScheduleServerPort  string            `json:"schedule_server_port"`
	PortAutoDetect      bool              `json:"port_auto_detect"`
	Properties          map[string]string `json:"properties"`
	Disks               YarnDisksConfig   `json:"disks"`
	ShimServer          string            `json:"shim_server"`
	CpuOverCommit       OverCommit        `json:"cpu_over_commit"`
}

// OverCommit set overcommit percent for resource
type OverCommit struct {
	Enable            bool                  `json:"enable"`
	OverCommitPercent float64               `json:"over_commit_percent"`
	Periods           []TimeRangeOverCommit `json:"periods"`
}

// TimeRangeOverCommit set overcommit percent for resource in specific time range
type TimeRangeOverCommit struct {
	Range             [2]times.SecondsInDay `json:"range"`
	OverCommitPercent float64               `json:"over_commit_percent"`
}

// YarnDisksConfig group disks config
type YarnDisksConfig struct {
	// RatioToCore translate disk space to core numbers
	RatioToCore      int64 `json:"ratio_to_core"`
	MultiDiskDisable bool  `json:"multi_disk_disable"`
	// DiskMinCapacityGb drop disks with little disk space
	DiskMinCapacityGb int64          `json:"disk_min_capacity_gb"`
	SpaceCheckEnabled bool           `json:"space_check_enabled"`
	SpaceCheckPeriod  times.Duration `json:"space_check_period"`
	// SpaceCheckReservedGb is used for checking disk space, it will start cleaning space if free disk space is less
	// than SpaceCheckReservedGb
	SpaceCheckReservedGb      int64   `json:"space_check_reserved_gb"`
	SpaceCheckReservedPercent float64 `json:"space_check_reserved_percent"`
	SpaceCleanDisable         bool    `json:"space_clean_disable"`
	// SpaceCleanJustData is enabled, it will just restart nodemanager pod to release /data space, and
	// do not care other disk partitions
	SpaceCleanJustData bool `json:"space_clean_just_data"`
	// OfflineExitedCleanDelay is used to clean nodemanager local or log path when offline pod exited for long time
	OfflineExitedCleanDelay times.Duration `json:"offline_exited_clean_delay"`
}

// RoundOffResource is used to format resource quantity,
// such as the origin memory is 1027Mi, we can get 1024Mi after rounding off, making memory 2 times of 512Mi
type RoundOffResource struct {
	CPUMilli float64 `json:"cpu_milli"`
	MemMB    float64 `json:"mem_mb"`
}

// RangeResource is used to check if the resource changed is available
// there is no need to update node resource when changed quantity is small.
type RangeResource struct {
	CPUMilli RangeState `json:"cpu_milli"`
	MemMB    RangeState `json:"mem_mb"`
}

// RangeState describe range resource to drop little changing
type RangeState struct {
	// Minimum is the range quantity
	Min float64 `json:"min"`
	// Maximum is the maxisum range quantity
	Max float64 `json:"max"`
	// Ratio used to calculate change range quantity
	Ratio float64 `json:"ratio"`
}

// ResourceIsolateConfig is the offline job quota limit configuration for resources
type ResourceIsolateConfig struct {
	Disable         bool            `json:"disable"`
	ResourceDisable map[string]bool `json:"resource_disable"`
	UpdatePeriod    times.Duration  `json:"update_period"`
	// disks need to set io weight
	DiskNames []string `json:"-"`
	// eni iface for eni network pods
	EniIface string `json:"-"`
	// normal iface for host network and global route network pods
	Iface              string            `json:"-"`
	CpuConfig          CpuIsolateConfig  `json:"cpu_config"`
	OnlineType         string            `json:"-"`
	OfflineType        string            `json:"-"`
	ExternalComponents []ComponentConfig `json:"external_components"`
}

// CpuIsolateConfig is the configuration for cpu isolation
type CpuIsolateConfig struct {
	// AutoDetect will enable bt feature if supported, and quota as the second choice.
	AutoDetect bool `json:"auto_detect"`
	// ManagePolicy assigns cpu manage policy
	ManagePolicy   string         `json:"manage_policy"`
	CpuSetConfig   CpuSetConfig   `json:"cpuset_config"`
	CpuQuotaConfig CpuQuotaConfig `json:"cpu_quota_config"`
	// KubeletStatic check if cpu manager policy for kubelet is static
	KubeletStatic bool `json:"-"`
}

// CpuSetConfig describe configs for cpuset isolation policy
type CpuSetConfig struct {
	// isolate online jobs with offline jobs
	EnableOnlineIsolate bool `json:"enable_online_isolate"`
	// cpu list, which offline job will not be assigned
	ReservedCpus string `json:"reserved_cpus"`
}

// CpuQuotaConfig describe configs for cpu quota isolation policy
type CpuQuotaConfig struct {
	// set offline job weights, just for quota policy
	OfflineShare *uint64 `json:"offline_share"`
}

// MetricsCollectConfig is the configuration for metrics collection
type MetricsCollectConfig struct {
	Node       MetricsNodeConfig      `json:"node"`
	Container  MetricsContainerConfig `json:"container"`
	Perf       MetricsPerfConfig      `json:"perf"`
	Rdt        MetricsRdtConfig       `json:"rdt"`
	Prometheus MetricsPrometheus      `json:"prometheus"`
}

// MetricsNodeConfig is the configuration for node metrics collection
type MetricsNodeConfig struct {
	CollectInterval times.Duration `json:"collect_interval"`
	SystemProcesses []string       `json:"system_processes"`
	OfflineType     string         `json:"-"`
	Devices         `json:",inline"`
}

// Devices group network and disk devices
type Devices struct {
	// Ifaces are the network interfaces, e.g. eth0, those not exist or down will be filter out
	// these ifaces will be assigned to metrics.node.ifaces
	IfacesWithProperty []string `json:"ifaces_xxx"`
	Ifaces             []string `json:"-"`
	// DiskNames are the disk names, e.g. sda, vda, those not exist will be filter out
	// these ifaces will be assigned to metrics.node.deviceNames
	DiskNames []string `json:"disk_names"`
}

// MetricsContainerConfig is the configuration for container metrics collection
type MetricsContainerConfig struct {
	Resources               []string       `json:"resources"`
	Cgroups                 []string       `json:"cgroups"`
	CollectInterval         times.Duration `json:"collect_interval"`
	MaxHousekeepingInterval times.Duration `json:"max_housekeeping_interval"`
}

// MetricsPerfConfig is the configuration for perf metrics collection
type MetricsPerfConfig struct {
	Disable         bool           `json:"disable"`
	CollectInterval times.Duration `json:"collect_interval"`
	CollectDuration times.Duration `json:"collect_duration"`
	IgnoredCgroups  []string       `json:"ignored_cgroups"`
}

// MetricsRdtConfig is the configuration for RDT metrics collection
type MetricsRdtConfig struct {
	Disable         bool           `json:"disable"`
	RdtCommand      string         `json:"rdt_command"`
	CollectInterval times.Duration `json:"collect_interval"`
	CollectDuration times.Duration `json:"collect_duration"`
	ExecuteInterval times.Duration `json:"execute_interval"`
}

// MetricsPrometheus describe how to collect prometheus metrics
type MetricsPrometheus struct {
	CollectInterval times.Duration `json:"collect_interval"`
	// if need to show these metrics with the prefix "caelus_"
	DisableShow bool              `json:"disable_show"`
	Items       []*PrometheusData `json:"items"`
}

// PrometheusData describe which metrics to collect or not collect
type PrometheusData struct {
	Address      string      `json:"address"`
	Collect      []string    `json:"collect"`
	NoCollect    []string    `json:"no_collect"`
	CollectMap   sets.String `json:"-"`
	NoCollectMap sets.String `json:"-"`
}

// CPIManagerConfig show the configuration for cpi detecting
type CPIManagerConfig struct {
	// I want this feature disabled by default
	Enable            bool           `json:"enable"`
	WindowDuration    times.Duration `json:"window_duration"`
	PrometheusAddrStr string         `json:"prometheus_addr"`
	PrometheusAddr    url.URL        `json:"-"`
	MaxJobSpecRange   times.Duration `json:"max_job_spec_range"`
}

// AlarmConfig group options to send alarm message
type AlarmConfig struct {
	Enable                 bool           `json:"enable"`
	Cluster                string         `json:"cluster"`
	MessageBatch           int            `json:"message_batch"`
	MessageDelay           times.Duration `json:"message_delay"`
	ChannelName            string         `json:"channel_name"`
	IgnoreAlarmWhenSilence bool           `json:"ignore_alarm_when_silence"`
	AlarmChannel           `json:"alarm_channel"`
}

// AlarmChannel struct is used to show alarm channel
type AlarmChannel struct {
	LocalAlarm  *LocalAlarm  `json:"local"`
	RemoteAlarm *RemoteAlarm `json:"remote"`
}

// LocalAlarm struct is used to describe local alarm body
type LocalAlarm struct {
	Executor string `json:"executor"`
}

// RemoteAlarm struct is used to describe remote alarm body
type RemoteAlarm struct {
	RemoteWebhook string `json:"remoteWebhook"`
	WeWorkWebhook string `json:"weWorkWebhook"`
}

// OnlineConfig show online job configuration
type OnlineConfig struct {
	Enable       bool              `json:"enable"`
	PidToCgroup  PidToCgroup       `json:"pid_to_cgroup"`
	Jobs         []OnlineJobConfig `json:"jobs"`
	CustomMetric CustomMetric      `json:"custom_metric"`
}

// PidToCgroup define online config of pid check
type PidToCgroup struct {
	// PidCheckInterval could be zero
	PidCheckInterval    times.Duration `json:"pids_check_interval"`
	CgroupCheckInterval times.Duration `json:"cgroup_check_interval"`
	BatchNum            int            `json:"batch_num"`
}

// OnlineJobConfig is the configuration of a online job
type OnlineJobConfig struct {
	Name string `json:"name"`
	// JobCommand is job's command expression
	Command string          `json:"command"`
	Metrics []OnlineMetrics `json:"metrics"`
}

// OnlineMetrics define metric config of online services
type OnlineMetrics struct {
	Name   string        `json:"name"`
	Source MetricsSource `json:"source"`
}

// MetricsSource define metrics source of online services
type MetricsSource struct {
	CheckInterval times.Duration `json:"check_interval"`
	// MetricsCommand is a command to get job's current metrics value, it must return the format data, like:
	// Its output is {"code":0,"msg":"success","data":[{"job_name":"","metric_name":"","key1":xx,"key2":xx,...}]}
	MetricsCommand []string `json:"metrics_command"`
	// if need to run chroot when executing metrics command
	CmdNeedChroot *bool `json:"cmd_need_chroot"`
	// MetricsURL is a url to get the job's metrics value, it must return the format data, like:
	// Its output is <slo>,<metrics>.
	MetricsURL string `json:"metrics_url"`
}

// CustomMetric define custom metric config
type CustomMetric struct {
	MetricServerAddr string         `json:"metric_server_addr"`
	CollectInterval  times.Duration `json:"collect_interval"`
}

// ParseJsonConfig parse json config
func ParseJsonConfig(configFile string) (*CaelusConfig, error) {
	if _, err := os.Stat(path.Join(RootFS, "/proc")); os.IsNotExist(err) {
		klog.V(2).Info("current namespace is host")
		util.InHostNamespace = true
	} else {
		klog.V(2).Info("current namespace is NOT host")
		util.InHostNamespace = false
	}

	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("open config file(%s) err: %v", configFile, err)
	}
	defer file.Close()

	caelus := &CaelusConfig{}
	if err := json.NewDecoder(file).Decode(caelus); err != nil {
		return nil, fmt.Errorf("parse config file(%s) to json err: %v", configFile, err)
	}

	if err := initJsonConfig(caelus); err != nil {
		return nil, err
	}
	data, err := json.Marshal(caelus)
	if err != nil {
		return nil, fmt.Errorf("marshal caelus config: %v", err)
	}
	klog.Infof("formated caelus json config: %s", string(data))
	return caelus, nil
}

func initJsonConfig(caelus *CaelusConfig) error {
	initCheckPointConfig(&caelus.CheckPoint)
	initTaskType(&caelus.TaskType)
	initNodeResourceConfig(&caelus.NodeResource, &caelus.TaskType)
	initMetricsConfig(&caelus.Metrics, &caelus.TaskType)
	initPredictConfigs(caelus.Predicts)
	initResourceIsolateConfig(&caelus.ResourceIsolate, &caelus.Metrics.Node, &caelus.TaskType, &caelus.K8sConfig)
	initAlarmConfig(&caelus.Alarm)
	initCPIManagerConfig(&caelus.CpiManager)
	initOnlineConfig(&caelus.Online)
	initDiskQuotaManager(&caelus.DiskQuota)
	return nil
}

func initCheckPointConfig(config *CheckPointConfig) {
	if len(config.CheckPointDir) == 0 {
		config.CheckPointDir = defaultCheckPointDir
	}
	if len(config.NodeResourceKey) == 0 {
		config.NodeResourceKey = defaultNodeResourceCheckPointKey
	}
}

func initTaskType(config *TaskTypeConfig) {
	if !AvailableOnlineTaskType.Has(config.OnlineType) {
		klog.Fatalf("invalid online type: %s, must in: %v", config.OnlineType, AvailableOnlineTaskType)
	}
	if !AvailableOfflineTaskType.Has(config.OfflineType) {
		klog.Fatalf("invalid offline type: %s, must in: %v", config.OfflineType, AvailableOfflineTaskType)
	}
}

func initNodeResourceConfig(config *NodeResourceConfig, taskType *TaskTypeConfig) {
	if config.Disable {
		return
	}

	if config.UpdateInterval.Seconds() == 0 {
		config.UpdateInterval = defaultNodeUpdateInterval
	}

	config.OfflineType = taskType.OfflineType
	if OfflineOnYarn(taskType) {
		if len(config.YarnConfig.NMServer) == 0 {
			klog.Fatal("node resource config for yarn has no nodemanager server address")
		}
		if len(config.YarnConfig.ScheduleServerPort) == 0 {
			config.YarnConfig.ScheduleServerPort = defaultYarnScheduleServerPort
		}
		if config.YarnConfig.CpuOverCommit.Enable {
			if config.YarnConfig.CpuOverCommit.OverCommitPercent < 1.0 {
				config.YarnConfig.CpuOverCommit.OverCommitPercent = defaultCpuOverCommitPercent
			}
			for _, p := range config.YarnConfig.CpuOverCommit.Periods {
				if p.Range[0] >= p.Range[1] {
					klog.Fatalf("bad overcommit period config")
				}
				if p.OverCommitPercent < 1.0 {
					klog.Fatalf("must specific overcommit percent(>=1.0) in specific time range")
				}
			}
		}
	}

	if config.YarnConfig.Disks.RatioToCore == 0 {
		config.YarnConfig.Disks.RatioToCore = defaultDisksToCore
	}

	if config.YarnConfig.Disks.DiskMinCapacityGb == 0 {
		config.YarnConfig.Disks.DiskMinCapacityGb = defaultMinDiskCapacity
	}
	if config.YarnConfig.Disks.SpaceCheckPeriod.Seconds() == 0 {
		config.YarnConfig.Disks.SpaceCheckPeriod = defaultDiskSpaceCheckPeriod
	}

	its := config.Silence.Periods
	for i := 0; i < len(its); i++ {
		if its[i][0] >= its[i][1] {
			klog.Fatalf("bad silence period: %v", its[i])
		}
		if i != 0 && its[i-1][1] >= its[i][0] {
			klog.Fatalf("bad silence period: %v and %v should be merged", its[i-1], its[i])
		}
	}
}

func initMetricsConfig(config *MetricsCollectConfig, taskType *TaskTypeConfig) {
	if config.Node.CollectInterval.Seconds() == 0 {
		config.Node.CollectInterval = defaultMetricsInterval
	}
	config.Node.OfflineType = taskType.OfflineType
	config.Node.DiskNames = existDisk(config.Node.DiskNames)
	var ifaces []string
	for _, f := range config.Node.IfacesWithProperty {
		index := strings.Index(f, "_")
		if index == -1 {
			ifaces = append(ifaces, f)
		} else {
			ifaces = append(ifaces, f[:index])
		}
	}
	config.Node.Ifaces = existAndUps(ifaces)

	if config.Container.MaxHousekeepingInterval.Seconds() == 0 {
		config.Container.MaxHousekeepingInterval = defaultMetricsInterval
	}
	if config.Container.CollectInterval.Seconds() == 0 {
		config.Container.CollectInterval = defaultMetricsInterval
	}
	if len(config.Container.Cgroups) == 0 {
		config.Container.Cgroups = []string{defaultContainerCgroups}
	}

	if !config.Perf.Disable {
		if config.Perf.CollectInterval.Seconds() == 0 {
			config.Perf.CollectInterval = defaultMetricsInterval
		}
		if config.Perf.CollectDuration.Seconds() == 0 {
			config.Perf.CollectDuration = defaultPerfDuration
		}
	}

	if !config.Rdt.Disable {
		if len(config.Rdt.RdtCommand) == 0 {
			config.Rdt.RdtCommand = defaultRdtCommand
		}
		if config.Rdt.CollectInterval.Seconds() == 0 {
			config.Rdt.CollectInterval = defaultMetricsInterval
		}
		if config.Rdt.CollectDuration.Seconds() == 0 {
			config.Rdt.CollectDuration = defaultRdtDuration
		}
		if config.Rdt.ExecuteInterval.Seconds() == 0 {
			config.Rdt.ExecuteInterval = defaultRdtInterval
		}
	}

	if config.Prometheus.CollectInterval.Seconds() == 0 {
		config.Prometheus.CollectInterval = defaultMetricsInterval
	}
	// translating slices to map struct
	for _, pro := range config.Prometheus.Items {
		pro.CollectMap = sets.NewString()
		for _, item := range pro.Collect {
			pro.CollectMap.Insert(item)
		}
		pro.NoCollectMap = sets.NewString()
		for _, item := range pro.NoCollect {
			pro.NoCollectMap.Insert(item)
		}
	}
}

func initResourceIsolateConfig(config *ResourceIsolateConfig, nodeConfig *MetricsNodeConfig,
	taskType *TaskTypeConfig, k8s *K8sConfig) {
	if config.Disable {
		return
	}

	for _, f := range nodeConfig.IfacesWithProperty {
		if strings.Contains(f, "_eni") {
			index := strings.Index(f, "_")
			config.EniIface = f[:index]
		} else {
			index := strings.Index(f, "_")
			config.Iface = f[:index]
		}
	}
	for _, d := range nodeConfig.DiskNames {
		config.DiskNames = append(config.DiskNames, d)
	}

	// assign task type
	config.OnlineType = taskType.OnlineType
	config.OfflineType = taskType.OfflineType
	// filter out not existing disks
	config.DiskNames = existDisk(config.DiskNames)

	if config.UpdatePeriod.Seconds() == 0 {
		config.UpdatePeriod = defaultResourceIsolateInterval
	}

	initCpuManagePolicy(&config.CpuConfig, k8s.KubeletRootDir)
}

func initCpuManagePolicy(config *CpuIsolateConfig, kubeletRootDir string) {
	if config.AutoDetect {
		if cgroup.CPUOfflineSupported() {
			config.ManagePolicy = CpuManagePolicyBT
		} else {
			config.ManagePolicy = CpuManagePolicyQuota
		}
		klog.Infof("cpu isolate auto detect is enabled, chosen manage policy is: %s", config.ManagePolicy)
	}
	if config.ManagePolicy == CpuManagePolicyAdaptive {
		if !cgroup.CPUOfflineSupported() {
			klog.Infof("BT not supported, using %s instead of %s", CpuManagePolicyQuota, config.ManagePolicy)
			config.ManagePolicy = CpuManagePolicyQuota
		}
	}

	if !AvailableCpuManagePolicy.Has(config.ManagePolicy) {
		klog.Fatalf("invalid cpu manage policy: %s", config.ManagePolicy)
	}

	config.KubeletStatic = false
	cpuMnFile := getCpuManagerFile(kubeletRootDir)
	if policyBytes, err := ioutil.ReadFile(cpuMnFile); err != nil {
		klog.Errorf("cpu manager file(%s) read failed, please input the right kubelet root-dir: %v", cpuMnFile, err)
	} else {
		if strings.Contains(string(policyBytes), "static") {
			// cpu manager policy is static
			config.KubeletStatic = true
		}
	}

	// if enable cpuset policy with isolating online jobs, cpu manager policy for kubelet should not be static
	if config.KubeletStatic &&
		config.ManagePolicy == CpuManagePolicySet && config.CpuSetConfig.EnableOnlineIsolate {
		klog.Fatalf("cpu manager policy is static, conflicted with cpuset policy and isolating online jobs")
	}
}

func getCpuManagerFile(rootDir string) string {
	if len(rootDir) == 0 {
		klog.Warning("kubelet root directory is empty, using default /data")
		rootDir = "/data"
	}
	if !util.InHostNamespace {
		klog.Warning("adding non-host namespace prefix for kubelet root dir")
		rootDir = path.Join(RootFS, rootDir)

		// if the directory is soft link in non-host namespace,
		// the files under the directory are not found, for it links the path in host namespace
		// such as, /rootfs/data/kubelet_root_dir -> /data9/kubelet_root_dir
		// if the directory has no link, the Readlink syscall still returns error, so no need to check error
		linkDir, _ := os.Readlink(rootDir)
		if len(linkDir) != 0 {
			klog.Warningf("%s is soft link, new path: %s", rootDir, linkDir)
			rootDir = path.Join(RootFS, linkDir)
		}
	}

	return path.Join(rootDir, "cpu_manager_state")
}

func initAlarmConfig(config *AlarmConfig) {
	if !config.Enable {
		return
	}

	if len(config.ChannelName) == 0 {
		config.ChannelName = AlarmTypeLocal
	}
	if !AvailableAlarmType.Has(config.ChannelName) {
		klog.Fatalf("invalid alarm channel, should be: %v", AvailableAlarmType)
	}

	if config.MessageBatch == 0 {
		config.MessageBatch = defaultAlarmMessageBatch
	}
	if config.MessageDelay.Seconds() == 0 {
		config.MessageDelay = defaultAlarmMessageDelay
	}
}

// initCPIManagerConfig used to check cpi manager config
func initCPIManagerConfig(config *CPIManagerConfig) {
	if config.WindowDuration.Seconds() == 0 {
		config.WindowDuration = defaultCPIWindowDuration
	}

	u, err := url.Parse(config.PrometheusAddrStr)
	if err != nil {
		klog.Fatalf("invalid prometheus_addr %s, err: %v", config.PrometheusAddrStr, err)
	}
	config.PrometheusAddr = *u

	if config.MaxJobSpecRange.Seconds() == 0 {
		config.MaxJobSpecRange = defaultCPIMaxJobSpecRange
	}
}

// initOnlineConfig initialize online job default configuration
func initOnlineConfig(config *OnlineConfig) {
	if config.PidToCgroup.CgroupCheckInterval.Seconds() == 0 {
		config.PidToCgroup.CgroupCheckInterval = defaultOnlineCheckInterval
	}
	if config.PidToCgroup.BatchNum == 0 {
		config.PidToCgroup.BatchNum = defaultOnlineBatchNumber
	}
	for i, job := range config.Jobs {
		for j, metric := range job.Metrics {
			if metric.Source.CheckInterval.Seconds() == 0 {
				config.Jobs[i].Metrics[j].Source.CheckInterval = defaultMetricsRequestInterval
			}
			if len(metric.Source.MetricsURL) != 0 && len(metric.Source.MetricsCommand) != 0 {
				klog.Fatal("metric request url and command must be set only one")
			}
			if len(metric.Source.MetricsCommand) != 0 {
				if metric.Source.CmdNeedChroot == nil {
					var defaultChroot bool = true
					config.Jobs[i].Metrics[j].Source.CmdNeedChroot = &defaultChroot
					klog.Warning("chroot before executing command is set true")
				}
			}
		}
	}
}

// parsePercentage parse the percent string value
func parsePercentage(input string) (float64, error) {
	if len(input) == 0 {
		return 0, nil
	}
	value, err := strconv.ParseFloat(strings.TrimRight(input, "%"), 64)
	if err != nil {
		return 0, err
	}
	return value / 100, nil
}

// OfflineOnYarn check if offline job is running on YARN
func OfflineOnYarn(config *TaskTypeConfig) bool {
	if config.OfflineType == OfflineTypeYarnOnk8s {
		return true
	}
	return false
}

// existDisk choose normal disks from the given diskNames
func existDisk(diskNames []string) []string {
	var exists []string
	devDir := "/dev"
	if !util.InHostNamespace {
		devDir = path.Join(RootFS, devDir)
	}
	for _, disk := range diskNames {
		p := path.Join(devDir, disk)
		_, err := os.Stat(p)
		if err == nil {
			exists = append(exists, disk)
		} else {
			klog.Errorf("the disk %s is not invalid", disk)
		}
	}
	return exists
}

// existAndUps choose ifaces in up state
func existAndUps(ifaces []string) []string {
	var ups []string
	for _, iface := range ifaces {
		nic, err := net.InterfaceByName(iface)
		if err != nil {
			klog.Errorf("the iface %s is not invalid", iface)
			continue
		}
		if nic.Flags&net.FlagUp == 1 {
			ups = append(ups, iface)
		} else {
			klog.Errorf("the iface %s is not up", iface)
		}
	}
	return ups
}

// AllResCompressible check if the resources are compressible
func AllResCompressible(res []string) bool {
	return CompressibleRes.HasAll(res...)
}
