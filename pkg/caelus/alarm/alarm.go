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

package alarm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/metrics"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util"

	"github.com/parnurzeal/gorequest"
	"k8s.io/klog/v2"
)

var alarmManager *Manager

// SendAlarm is global function to receive alarm message
func SendAlarm(msg string) {
	go func() {
		if alarmManager == nil || !alarmManager.Enable {
			return
		}
		if alarmManager.ojg != nil && alarmManager.ojg.OfflineScheduleDisabled() {
			jobs, err := alarmManager.ojg.GetOfflineJobs()
			if err != nil {
				klog.Errorf("failed get offline jobs: %v", err)
			} else if len(jobs) == 0 {
				// omit alarm message if schedule disabled and no offline job is running
				return
			}
		}
		metrics.AlarmCounterInc()
		klog.V(4).Infof("receiving alarm: %s", msg)
		select {
		case alarmManager.msgBuffers <- alarmMsg{ts: time.Now(), msg: msg}:
		default:
			klog.Errorf("alarm channel is full, dropping msg: %s", msg)
		}
	}()
}

type offlineJobGetter interface {
	// GetOfflineJobs return current offline job list
	GetOfflineJobs() ([]types.OfflineJobs, error)
	// OfflineScheduleDisabled return true if schedule disabled for offline jobs
	OfflineScheduleDisabled() bool
}

// Manager is used to send alarm message
type Manager struct {
	types.AlarmConfig
	localIP string
	delayer *time.Timer
	alarms  struct {
		messages map[string][]time.Time
		sync.RWMutex
	}
	msgBuffers chan alarmMsg
	// different sending channel
	send sendChannel
	ojg  offlineJobGetter
}

type alarmMsg struct {
	msg string
	ts  time.Time
}

// NewManager returns an instance of alarm manager
func NewManager(cfg *types.AlarmConfig, ojg offlineJobGetter) *Manager {
	alarmManager = &Manager{
		AlarmConfig: *cfg,
		localIP:     util.NodeIP(),
		alarms: struct {
			messages map[string][]time.Time
			sync.RWMutex
		}{messages: make(map[string][]time.Time)},

		msgBuffers: make(chan alarmMsg, 100),
		ojg:        ojg,
	}

	if alarmManager.Enable {
		if cfg.ChannelName == types.AlarmTypeLocal {
			alarmManager.send = newLocalScript(cfg.LocalAlarm)
		} else {
			alarmManager.send = newRemoteAlarm(cfg.RemoteAlarm)
		}
	}

	return alarmManager
}

// Name return module name
func (a *Manager) Name() string {
	return "ModuleAlarm"
}

// Run is the main loop to send alarm messages
func (a *Manager) Run(stop <-chan struct{}) {
	if !a.Enable {
		klog.Warningf("alarm manager is not enabled")
		return
	}
	klog.Infof("alarm manager running")

	go func() {
		for {
			select {
			case <-stop:
				return
			case msg := <-a.msgBuffers:
				// if ignoring warning is true, no need alarm when entry silence mode
				if a.IgnoreAlarmWhenSilence && util.SilenceMode {
					klog.Warningf("entry silence mode and ignore warning is true, drop the msg: %s", msg.msg)
					return
				}

				func() {
					a.alarms.Lock()
					defer a.alarms.Unlock()
					a.alarms.messages[msg.msg] = append(a.alarms.messages[msg.msg], msg.ts)
					msgLen := 0
					for _, tss := range a.alarms.messages {
						msgLen += len(tss)
					}
					if msgLen >= a.MessageBatch {
						body := &AlarmBody{
							IP:       a.localIP,
							Cluster:  a.Cluster,
							AlarmMsg: a.getAlarmMessages(),
						}
						klog.Infof("sending alarms: %v", body.AlarmMsg)
						// send alarm can fork another thread
						go a.send.sendMessage(body)
						// clear messages already sent
						a.alarms.messages = make(map[string][]time.Time)
					} else {
						if a.delayer == nil {
							klog.V(4).Infof("timer is nil, sending after %v", a.MessageDelay)
							a.delayer = time.AfterFunc(a.MessageDelay.TimeDuration(), a.delaySend)
						} else {
							klog.V(4).Infof("timer will reset, sending after %v", a.MessageDelay)
							a.delayer.Reset(a.MessageDelay.TimeDuration())
						}
					}
				}()
			}
		}
	}()
}

// delaySend send alarm message asynchronously
func (a *Manager) delaySend() {
	a.alarms.Lock()
	defer a.alarms.Unlock()
	if len(a.alarms.messages) == 0 {
		return
	}

	body := &AlarmBody{
		IP:       a.localIP,
		Cluster:  a.Cluster,
		AlarmMsg: a.getAlarmMessages(),
	}
	klog.V(4).Infof("delay sending alarms: %v", body.AlarmMsg)
	go a.send.sendMessage(body)
	a.alarms.messages = make(map[string][]time.Time)
}

func (a *Manager) getAlarmMessages() []string {
	var messages []string
	for msg, tss := range a.alarms.messages {
		if len(tss) == 1 {
			messages = append(messages, fmt.Sprintf("%s[%s]", msg, tss[0].Format("15:04:05")))
		} else {
			messages = append(messages, fmt.Sprintf("%s[%s - %s, %d]",
				msg, tss[0].Format("15:04:05"), tss[len(tss)-1].Format("15:04:05"), len(tss)))
		}
	}
	return messages
}

// sendChannel is the interface to send alarm message
type sendChannel interface {
	sendMessage(body *AlarmBody) error
}

// LocalAlarm returns the info of local alarm struct
type localAlarm struct {
	executor string
}

func newLocalScript(config *types.LocalAlarm) sendChannel {
	return &localAlarm{
		executor: config.Executor,
	}
}

func (s *localAlarm) sendMessage(body *AlarmBody) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		klog.Errorf("invalid alarm body format: %v", err)
		return err
	}

	cmd := exec.Command(s.executor, string(bodyBytes))
	out, err := cmd.Output()
	if err != nil {
		klog.Errorf("local alarm execute error, output: %s, err: %v", string(out), err)
	}

	return nil
}

// RemoteAlarm return the info of remote alarm struct
type remoteAlarm struct {
	remoteWebhook string
	weWorkWebhook string
}

// WeWorkMessage is the format of wework message
type WeWorkMessage struct {
	MessageType string   `json:"msgtype"`
	Markdown    markdown `json:"markdown,omitempty"`
}

type markdown struct {
	Content string `json:"content"`
}

// response shows remote alarm response data
type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// AlarmBody defines alarm body sending to script or api
type AlarmBody struct {
	IP       string   `json:"ip"`
	Cluster  string   `json:"cluster"`
	AlarmMsg []string `json:"alarmMsg"`
}

func newRemoteAlarm(config *types.RemoteAlarm) sendChannel {
	return &remoteAlarm{
		remoteWebhook: config.RemoteWebhook,
		weWorkWebhook: config.WeWorkWebhook,
	}
}

func (r *remoteAlarm) sendMessage(body *AlarmBody) error {
	var err error
	if len(r.weWorkWebhook) != 0 {
		err = r.sendMessageToWeWork(body)
	} else {
		err = r.sendMessageToRemote(body)
	}
	if err != nil {
		return fmt.Errorf("send alarm msg err: %v", err)
	}
	return nil
}

func (r *remoteAlarm) sendMessageToRemote(body *AlarmBody) error {
	if len(r.remoteWebhook) == 0 {
		return nil
	}
	rsp, bodyBytes, errs := gorequest.New().
		Post(r.remoteWebhook).
		Set("Content-Type", "application/json").
		Send(body).
		EndBytes()
	if len(errs) > 0 {
		var errMsgs []string
		for _, e := range errs {
			errMsgs = append(errMsgs, e.Error())
		}
		return fmt.Errorf("send alarm to remote err: %v", errMsgs)
	} else if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("send alarm to remote, status no ok: %s, resp: %s", rsp.Status, string(bodyBytes))
	}

	rspBody := &response{}
	if err := json.Unmarshal(bodyBytes, rspBody); err != nil {
		return fmt.Errorf("unmarshal json err: %v", err)
	}
	if rspBody.Code != 0 {
		return fmt.Errorf("send alarm to remote failed: Code %d, Message: %s", rspBody.Code, rspBody.Message)
	}

	return nil
}

func (r *remoteAlarm) sendMessageToWeWork(body *AlarmBody) error {
	if len(r.weWorkWebhook) == 0 {
		return nil
	}
	content := generateContent(body)
	request := gorequest.New()
	resp, _, errs := request.Post(r.weWorkWebhook).
		Set("Content-Type", "application/json").
		Send(content).
		End()
	if errs != nil {
		return fmt.Errorf("failed to send message to WeWork: %v", errs)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to send message to WeWork, status code is bad: %v", resp.StatusCode)
	}

	return nil
}

func generateContent(body *AlarmBody) *WeWorkMessage {
	content := fmt.Sprintf("告警IP(%s)：%s\n\n告警信息：%s", body.Cluster, body.IP, body.AlarmMsg)
	msg := &WeWorkMessage{
		MessageType: "markdown",
		Markdown: markdown{
			Content: content,
		},
	}

	return msg
}
