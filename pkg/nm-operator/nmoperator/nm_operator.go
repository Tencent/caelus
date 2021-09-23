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

package nmoperator

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tencent/caelus/pkg/cadvisor"
	"github.com/tencent/caelus/pkg/nm-operator/hadoop"
	"github.com/tencent/caelus/pkg/nm-operator/types"
	"github.com/tencent/caelus/pkg/nm-operator/util"
	global "github.com/tencent/caelus/pkg/types"

	"github.com/emicklei/go-restful"
	"github.com/parnurzeal/gorequest"
	"k8s.io/klog"
)

const (
	defaultUser = "root"
	basePath    = "/v1/nm"
	stateNMUp   = "UP"
	stateNMDown = "DOWN"

	// EnvPidFile show nodemanager pid file
	EnvPidFile = "PID_FILE"
	// EnvCgroupPath show cgroup path
	EnvCgroupPath = "CGROUP_PATH"
	// EnvHadoopYarnHomd show nodemanager working path
	EnvHadoopYarnHomd = "HADOOP_YARN_HOME"
	// EnvUser show nodemanager working user in linux
	EnvUser = "USER"
	// EnvGroup show nodemanager working group in linux
	EnvGroup = "GROUP"
)

// RegisterNMOperatorService register nodemanager operation service
func RegisterNMOperatorService(enableCadvisor bool, container *restful.Container) {
	op := NewNMOperator(enableCadvisor)

	ws := new(restful.WebService)
	ws.Path(basePath).Produces(restful.MIME_JSON)

	// check if the nodemanager process is running
	ws.Route(ws.GET("/status").To(op.GetStatus))

	// resource capacity
	ws.Route(ws.GET("/capacity").To(op.GetCapacity))
	ws.Route(ws.POST("/capacity").To(op.SetCapacity))

	// yarn job list
	ws.Route(ws.GET("/containers").To(op.GetContainers))
	ws.Route(ws.GET("/containerstats").To(op.GetContainersState))

	// kill container
	ws.Route(ws.POST("/kill").To(op.KillContainers))

	// nodemanger configuration
	ws.Route(ws.GET("/property").To(op.GetProperty))
	ws.Route(ws.POST("/property").To(op.SetProperty))

	// start or stop nodemanager process
	ws.Route(ws.POST("/start").To(op.StartNodeManager))
	ws.Route(ws.POST("/stop").To(op.StopNodeManager))

	// disable nodemanager accepting new jobs
	ws.Route(ws.POST("/schedule/disable").To(op.ScheduleDisable))
	ws.Route(ws.POST("/schedule/enable").To(op.ScheduleEnable))

	// update nodemanger resource both for local config and resourcemanager dynamically
	ws.Route(ws.POST("/capacity/update").To(op.UpdateCapacity))
	ws.Route(ws.POST("/capacity/update/force").To(op.ForceUpdateCapacity))

	container.Add(ws)
}

// NMOperator group options for nodemanager operation
type NMOperator struct {
	// user and group showing which identity to execute yarn command
	user             string
	group            string
	cadvisorManager  cadvisor.Cadvisor
	yarnBin          string
	yarnDaemonBin    string
	scheduleDisabled bool
}

// NewNMOperator return a new nodemanager operator instance
func NewNMOperator(enableCadvisor bool) *NMOperator {
	user := os.Getenv(EnvUser)
	if len(user) == 0 {
		user = defaultUser
	}
	group := os.Getenv(EnvGroup)
	if len(group) == 0 {
		group = defaultUser
	}

	if err := util.InitCgroup(user, group, EnvCgroupPath); err != nil {
		klog.Fatalf("init cgroup err: %v", err)
	}

	var cManager cadvisor.Cadvisor
	if enableCadvisor {
		cManager = cadvisor.NewCadvisorManager(types.CadvisorParameters, types.CgroupPath)
		err := cManager.Start()
		if err != nil {
			klog.Fatalf("cadvisor start err: %v", err)
		}
	}

	yarnHome := os.Getenv(EnvHadoopYarnHomd)
	if len(yarnHome) == 0 {
		klog.Errorf("env %s is nil", EnvHadoopYarnHomd)
	}

	return &NMOperator{
		user:             user,
		group:            group,
		cadvisorManager:  cManager,
		yarnBin:          fmt.Sprintf("%s/bin/yarn", yarnHome),
		yarnDaemonBin:    fmt.Sprintf("%s/sbin/yarn-daemon.sh", yarnHome),
		scheduleDisabled: false,
	}
}

// StartNodeManager start nodemanager
func (n *NMOperator) StartNodeManager(request *restful.Request, response *restful.Response) {
	klog.Info("start nodemanager request")
	if err := util.ExecuteYarnCMD([]string{n.yarnDaemonBin, "start", "nodemanager"}, n.user); err != nil {
		msg := fmt.Sprintf("start Nodemanager err: %v", err)
		klog.Error(msg)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, msg))
		return
	}

	response.WriteAsJson("Nodemanager started")
}

// StopNodeManager stop nodemanager
func (n *NMOperator) StopNodeManager(request *restful.Request, response *restful.Response) {
	klog.Info("stop nodemanager request")
	if err := util.ExecuteYarnCMD([]string{n.yarnDaemonBin, "stop", "nodemanager"}, n.user); err != nil {
		msg := fmt.Sprintf("stop Nodemanager err: %v", err)
		klog.Error(msg)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, msg))
		return
	}

	response.WriteAsJson("Nodemanager stopped")
}

// GetStatus check if the nodemanager process is running, by checking pid file under /proc
func (n *NMOperator) GetStatus(request *restful.Request, response *restful.Response) {
	var pid int
	nmStatus := global.NMStatus{
		Pid:         0,
		State:       stateNMDown,
		Description: "",
	}

	// the nodemanager process could record the pid into a fixed file
	pidfile := os.Getenv(EnvPidFile)
	if pidfile == "" {
		pidfile = "/tmp/yarn--nodemanager.pid"
	}

	dat, err := ioutil.ReadFile(pidfile)
	if err != nil {
		nmStatus.Description = fmt.Sprintf("Failed to open pid file %s: %v", pidfile, err)
		response.WriteAsJson(nmStatus)
		return
	}
	pid, err = strconv.Atoi(strings.Trim(string(dat), "\n\r\t "))
	if err != nil {
		nmStatus.Description = fmt.Sprintf("Failed to get pid from %s: %v", dat, err)
		response.WriteAsJson(nmStatus)
		return
	}
	nmStatus.Pid = pid

	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		nmStatus.Description = fmt.Sprintf("Failed to stat /proc/%d: %v", pid, err)
		response.WriteAsJson(nmStatus)
		return
	}
	dat, err = ioutil.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		nmStatus.Description = fmt.Sprintf("Failed to read /proc/%d/cmdline: %v", pid, err)
		response.WriteAsJson(nmStatus)
		return
	}
	if !strings.Contains(string(dat), "NodeManager") {
		nmStatus.Description = fmt.Sprintf("%d is not Nodemanager pid", pid)
		response.WriteAsJson(nmStatus)
		return
	}

	nmStatus.State = stateNMUp
	response.WriteAsJson(nmStatus)
	return
}

// GetCapacity get capacity
func (n *NMOperator) GetCapacity(request *restful.Request, response *restful.Response) {
	capacity, err := hadoop.GetCapacity()
	if err != nil {
		klog.Errorf("get capacity err: %v", err)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, err.Error()))
		return
	}

	response.WriteAsJson(&capacity)
}

// SetCapacity set capacity
func (n *NMOperator) SetCapacity(request *restful.Request, response *restful.Response) {
	klog.Info("set capacity request")
	_, code, err := setCapacity(request, response)
	if err != nil {
		response.WriteServiceError(code, restful.NewError(code, err.Error()))
	} else {
		response.WriteHeader(http.StatusOK)
	}
	return
}

// setCapacity return the following:
// - the capacity need to set
// - http status code
// - error
func setCapacity(request *restful.Request, response *restful.Response) (*global.NMCapacity, int, error) {
	capacity := &global.NMCapacity{}
	err := request.ReadEntity(capacity)
	if err != nil {
		klog.Errorf("invalid capacity data: %v", err)
		return capacity, http.StatusBadRequest, err
	}
	klog.Infof("set capacity to: %+v", capacity)

	err = hadoop.SetCapacity(*capacity)
	if err != nil {
		klog.Errorf("set capacity err: %v", err)
		return capacity, http.StatusInternalServerError, err
	}

	return capacity, http.StatusOK, nil
}

// GetProperty get property
func (n *NMOperator) GetProperty(request *restful.Request, response *restful.Response) {
	key := &global.ConfigKey{}
	var property global.ConfigProperty

	err := request.ReadEntity(key)
	if err != nil {
		response.WriteServiceError(http.StatusBadRequest, restful.NewError(http.StatusBadRequest, err.Error()))
		return
	}

	value := make(map[string]string)
	if key.ConfigAll {
		value, err = hadoop.GetAllConfig(key.ConfigFile)
		if err != nil {
			response.WriteServiceError(http.StatusInternalServerError,
				restful.NewError(http.StatusBadRequest, err.Error()))
			return
		}
	} else {
		value, err = hadoop.GetConfig(key.ConfigFile, key.ConfigKeys)
		if err != nil {
			response.WriteServiceError(http.StatusInternalServerError,
				restful.NewError(http.StatusBadRequest, err.Error()))
			return
		}
	}

	property.ConfigFile = key.ConfigFile
	property.ConfigProperty = value
	response.WriteAsJson(&property)
}

// SetProperty set property
func (n *NMOperator) SetProperty(request *restful.Request, response *restful.Response) {
	property := &global.ConfigProperty{}

	err := request.ReadEntity(&property)
	if err != nil {
		response.WriteServiceError(http.StatusBadRequest, restful.NewError(http.StatusBadRequest, err.Error()))
		return
	}
	klog.Infof("set property request: %+v", property)

	if !property.ConfigAdd && !property.ConfigDel {
		err = hadoop.SetConfig(property.ConfigFile, property.ConfigProperty)
		if err != nil {
			response.WriteServiceError(http.StatusInternalServerError,
				restful.NewError(http.StatusBadRequest, err.Error()))
			return
		}
	} else {
		err = hadoop.SetAddDelConfig(property.ConfigFile, property.ConfigProperty,
			property.ConfigAdd, property.ConfigDel)
		if err != nil {
			response.WriteServiceError(http.StatusInternalServerError,
				restful.NewError(http.StatusBadRequest, err.Error()))
			return
		}
	}

	response.WriteHeader(http.StatusOK)
}

// GetContainersState get given container state list
func (n *NMOperator) GetContainersState(request *restful.Request, response *restful.Response) {
	cids := &global.NMContainerIds{}

	if n.cadvisorManager == nil {
		klog.Errorf("cadvisor is disabled")
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, "cadvisor is disabled"))
		return
	}

	err := request.ReadEntity(cids)
	if err != nil {
		klog.Errorf("invalid container ids: %v", err)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, err.Error()))
		return
	}

	cstats := util.GetContainersState(cids.Cids, n.cadvisorManager)
	response.WriteAsJson(&cstats)
}

// GetContainers return container list
func (n *NMOperator) GetContainers(request *restful.Request, response *restful.Response) {
	// appid : amContainerId
	var apps = make(map[string]string)
	containers := &global.NMContainersWrapper{}

	handler := func(cacheAddress bool) error {
		NmWebAddress := hadoop.GetNodeManagerWebAddress(cacheAddress)
		url := "http://" + NmWebAddress + "/ws/v1/node/containers"
		resp, _, errs := gorequest.New().Get(url).Set("Accept", "application/json").
			EndStruct(containers)

		if len(errs) != 0 {
			klog.Errorf("GET %s failed: %s\n", url, errs[0].Error())
			return errs[0]
		}
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("GET %s status not ok: %d\n", url, resp.StatusCode)
			return fmt.Errorf("reponset status not ok: %d", resp.StatusCode)
		}
		return nil
	}

	// the NodeManager web address may be changed dynamically, so try again if request failed
	err := handler(true)
	if err != nil {
		klog.Errorf("get container list err, try again: %v", err)
		err = handler(false)
	}
	if err != nil {
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, err.Error()))
		return
	}

	for i := range containers.Container {
		appid := getAppIDFromContainerID(containers.Container[i].ID)
		containers.Container[i].AppID = appid
		apps[appid] = ""
	}

	if klog.V(3) {
		var buf bytes.Buffer
		for app, am := range apps {
			buf.WriteString(app)
			buf.WriteString(": ")
			buf.WriteString(am)
			buf.WriteString("\n")
		}
		klog.Infof("app list:\n%s", buf.String())
	}

	for app := range apps {
		am := getAmContainer(app)
		apps[app] = am
	}

	if klog.V(3) {
		var buf bytes.Buffer
		for app, am := range apps {
			buf.WriteString(app)
			buf.WriteString(": ")
			buf.WriteString(am)
			buf.WriteString("\n")
		}
		klog.Infof("app list:\n%s", buf.String())
	}

	for i := range containers.Container {
		if containers.Container[i].ID == apps[containers.Container[i].AppID] {
			containers.Container[i].IsAM = true
		}
	}

	response.WriteAsJson(containers)
}

// KillContainers evict container list by kill process group id
func (n *NMOperator) KillContainers(request *restful.Request, response *restful.Response) {
	cids := &global.NMContainerIds{}
	err := request.ReadEntity(cids)
	if err != nil {
		klog.Errorf("invalid container ids: %v", err)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, err.Error()))
		return
	}

	wait := sync.WaitGroup{}
	for _, cid := range cids.Cids {
		wait.Add(1)
		go func(cid string) {
			defer wait.Done()
			klog.Infof("killing nm container %+v", cid)
			gpid := util.GetContainerProcessGroupId(cid)
			if gpid <= 1 {
				klog.Infof("invalid process group pid(%d) for container %s", gpid, cid)
				return
			}
			// negative pid for kill is used to kill process group
			// see https://www.man7.org/linux/man-pages/man2/kill.2.html
			syscall.Kill(-gpid, syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			if !util.CheckPidAlive(gpid) {
				klog.Infof("container(%s) killed successfully", cid)
				return
			}
			syscall.Kill(-gpid, syscall.SIGKILL)
		}(cid)
	}
	if util.WaitTimeout(&wait, time.Minute) {
		klog.Warningf("killing running container timeout")
	}

	response.WriteHeader(http.StatusOK)
}

// ScheduleDisable disable nodemanager accepting new instances by setting resource capacity as zero
func (n *NMOperator) ScheduleDisable(request *restful.Request, response *restful.Response) {
	klog.Info("schedule disable request")
	err := util.ExecuteYarnUpdateNodeResource(n.yarnBin, n.user, "0", "0")
	if err != nil {
		klog.Errorf("disable NM schedule err: %v", err)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, err.Error()))
		return
	}

	n.scheduleDisabled = true
	response.WriteAsJson("NM schedule disabled")
}

// ScheduleEnable enable nodemanager accepting new instances by recovering resource capacity
func (n *NMOperator) ScheduleEnable(request *restful.Request, response *restful.Response) {
	klog.Info("schedule enable request")
	cap, err := hadoop.GetCapacity()
	if err != nil {
		msg := fmt.Sprintf("get capacity err when enable NM schedule: %v", err)
		klog.Error(msg)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, msg))
		return
	}

	memMb := fmt.Sprintf("%d", cap.MemoryMB)
	vCores := fmt.Sprintf("%d", cap.Vcores)
	err = util.ExecuteYarnUpdateNodeResource(n.yarnBin, n.user, memMb, vCores)
	if err != nil {
		msg := fmt.Sprintf("enable NM schedule err with capacity(memMb: %s, vCores: %s): %v", memMb, vCores, err)
		klog.Error(msg)
		response.WriteServiceError(http.StatusInternalServerError,
			restful.NewError(http.StatusInternalServerError, msg))
		return
	}

	n.scheduleDisabled = false
	response.WriteAsJson(fmt.Sprintf("NM schedule enabled with capacity(memMb: %s, vCores: %s)", memMb, vCores))
}

// UpdateCapacity update node capacity when node is in schedule enabled state
func (n *NMOperator) UpdateCapacity(request *restful.Request, response *restful.Response) {
	klog.Info("update capacity request")
	n.updateNodeCapacity(request, response, false)
}

// ForceUpdateCapacity update node capacity even when node is in schedule disabled state
func (n *NMOperator) ForceUpdateCapacity(request *restful.Request, response *restful.Response) {
	klog.Info("update capacity with force option request")
	n.updateNodeCapacity(request, response, true)
}

func (n *NMOperator) updateNodeCapacity(request *restful.Request, response *restful.Response, force bool) {
	capacity, code, err := setCapacity(request, response)
	if err != nil {
		response.WriteServiceError(code, restful.NewError(code, err.Error()))
		return
	}

	// notify master to update capacity
	if force || !n.scheduleDisabled {
		klog.Infof("execute update node resource to: +%v", capacity)
		err = util.ExecuteYarnUpdateNodeResource(n.yarnBin, n.user, fmt.Sprintf("%d", capacity.MemoryMB),
			fmt.Sprintf("%d", capacity.Vcores))
		if err != nil {
			klog.Errorf("update NM with capacity(%+v): %v", err, capacity)
			response.WriteServiceError(http.StatusInternalServerError,
				restful.NewError(http.StatusInternalServerError, err.Error()))
			return
		}
	}

	response.WriteHeader(http.StatusOK)
}

// container_e03_1505269670687_0001_01_000001 ->
// application_1505269670687_0001
func getAppIDFromContainerID(containerID string) string {
	tokens := strings.SplitN(containerID, "_", 5)
	if tokens == nil {
		return "Error spliting containerid"
	}
	tokens[1] = "application"
	tokens = tokens[1 : len(tokens)-1]
	return strings.Join(tokens, "_")
}

// GetContainerIDFromAmLogDir return container id
func getContainerIDFromAmLogDir(klogdir string) string {
	start := strings.Index(klogdir, "container_")
	if start < 0 {
		return ""
	}
	id := klogdir[start:]
	end := strings.Index(id, "/")
	if end < 0 {
		return id
	}
	return id[:end]
}

// GetAmContainer return AM container, which is more important
func getAmContainer(appID string) string {
	var AmID string

	handler := func(cacheAddress bool) error {
		rmAddr := hadoop.GetResourceManagerAddress(cacheAddress)
		rmAddrSlice := strings.Split(rmAddr, ",")

		requestOK := false
		for _, addr := range rmAddrSlice {
			url := "http://" + addr + "/ws/v1/cluster/apps/" + appID
			rmApp := types.RMAppWrapper{}
			_, _, errs := gorequest.New().Get(url).Set("Accept", "application/json").
				EndStruct(&rmApp)

			if len(errs) != 0 {
				klog.Errorf("GET %s failed: %s\n", url, errs[0].Error())
				continue
			}

			requestOK = true
			if len(rmApp.App.AmLogs) != 0 {
				AmID = getContainerIDFromAmLogDir(rmApp.App.AmLogs)
				return nil
			}
		}
		if !requestOK {
			return fmt.Errorf("request RM address failed")
		} else {
			return nil
		}
	}

	// the RM address may be changed dynamically, so try again if request failed
	err := handler(true)
	if err != nil {
		klog.Errorf("get AM container err, try again: %v", err)
		handler(false)
	}

	return AmID
}
