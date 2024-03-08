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

package rdt

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
)

var (
	rdtCommand string
	rdtCmdLock sync.Mutex
)

// RdtFeature show which feature supported
type RdtFeature struct {
	MonitorLLC       bool
	MonitorMBM       bool
	MonitorIPC       bool
	MonitorCacheMiss bool
	AllocateCAT      bool
}

// RdtData implements data collected from rdt tool
type RdtData struct {
	Cores     string
	IPC       float64
	Missees   float64
	LLC       float64
	MBL       float64
	MBR       float64
	Timestamp time.Time
}

// InitRdtCommand init rdt command
func InitRdtCommand(rdtCmd string) error {
	file, err := os.Stat(rdtCmd)
	if err != nil {
		klog.Errorf("rdt command file(%s) stat err: %v", rdtCmd, err)
		return err
	}
	if (file.Mode().Perm() & (1 << 6)) == 0 {
		klog.Errorf("rdt command file(%s) not executable", rdtCmd)
		return fmt.Errorf("rdt command file not executable")
	}

	rdtCommand = rdtCmd
	return nil
}

// runRdtCommand run rdt command
func runRdtCommand(args []string) (string, error) {
	// can not run in parallel at the same time
	rdtCmdLock.Lock()
	defer rdtCmdLock.Unlock()

	cmd := exec.Command(rdtCommand, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		klog.Errorf("run rdt cmd(%s %s) err: %v, stderr: %s, stdout: %s",
			rdtCommand, args, err, stderr.String(), out.String())
	}

	return out.String(), err
}

// SupportRdtFunction check which feature supported
func SupportRdtFunction() *RdtFeature {
	supported := &RdtFeature{
		MonitorLLC:       true,
		MonitorMBM:       false,
		MonitorIPC:       false,
		MonitorCacheMiss: false,
		AllocateCAT:      false,
	}
	out, err := runRdtCommand([]string{"-d"})
	if err != nil {
		if strings.Contains(out, "Monitoring capability not supported") {
			klog.Warning("rdt monitore capability not supported")
		} else {
			klog.Errorf("check rdt supported err: %v", err)
		}
		return supported
	}

	if strings.Contains(out, "LLC Occupancy") {
		supported.MonitorLLC = true
	}
	if strings.Contains(out, "Total Memory Bandwidth") {
		supported.MonitorMBM = true
	}
	if strings.Contains(out, "") {
		supported.MonitorCacheMiss = true
	}
	if strings.Contains(out, "Instructions/Clock") {
		supported.MonitorIPC = true
	}
	if strings.Contains(out, "") {
		supported.AllocateCAT = true
	}

	return supported
}

// StaticMonitor monitor for the static duration, and output results
// monitors format, such as: all:[0,2,4-10] or all:0,2,4
// if input like all:[0,2], will output:
//
//	CORE         IPC      MISSES     LLC[KB]   MBL[MB/s]   MBR[MB/s]
//	  0,2        0.55         12k     36176.0        47.8      1463.1
//
// or will output
//
//	CORE         IPC      MISSES     LLC[KB]   MBL[MB/s]   MBR[MB/s]
//	 0        0.55         12k     36176.0        47.8      1463.1
//	 2        0.55         12k     36176.0        47.8      1463.1
func StaticMonitor(monitors []string, monInterval, monTime time.Duration) ([]RdtData, error) {
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors found")
	}
	monitorsStr := strings.Join(monitors, ";")
	args := []string{"-m", monitorsStr, "-i", strconv.FormatInt(int64(monInterval.Seconds())*10, 10),
		"-t", strconv.FormatInt(int64(monTime.Seconds()), 10), "--static-mode"}
	out, err := runRdtCommand(args)
	if err != nil {
		klog.Errorf("run static monitor err: %v", err)
		return nil, err
	}

	retItems := []string{}
	startRecord := false
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		lineStr := scanner.Text()
		if strings.Contains(lineStr, "CORE") {
			startRecord = true
			if len(retItems) != 0 {
				retItems = []string{}
			}
			continue
		}
		if startRecord {
			retItems = append(retItems, lineStr)
		}
	}

	var rdtRets []RdtData
	for _, item := range retItems {
		rdtRet := RdtData{}
		values := strings.Split(item, " ")
		var value []float64
		coreReaded := false
		for _, v := range values {
			if v != "" {
				if !coreReaded {
					rdtRet.Cores = v
					coreReaded = true
				} else {
					vv, err := strconv.ParseFloat(v, 64)
					if err != nil {
						vq := resource.MustParse(v)
						vv = float64((&vq).Value())
					}
					value = append(value, vv)
				}
			}
		}
		rdtRet.IPC = value[0]
		rdtRet.Missees = value[1]
		if len(value) > 2 {
			rdtRet.LLC = value[2]
		}
		if len(value) > 4 {
			rdtRet.MBL = value[3]
			rdtRet.MBR = value[4]
		}

		rdtRet.Timestamp = time.Now()
		rdtRets = append(rdtRets, rdtRet)
	}

	return rdtRets, nil
}

// MonitorReset reset rdt
func MonitorReset() error {
	args := []string{"-r", "-t", "1"}
	_, err := runRdtCommand(args)

	return err
}

// TODO
func AllocateLLC() {

}
