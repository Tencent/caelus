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

package netio

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
	"github.com/tencent/caelus/pkg/caelus/util/machine"

	"github.com/chenchun/ipset"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/exec"
)

const (
	rootMinor    = 1
	onlineMinor  = 2
	offlineMinor = 3

	onlinePrio  = 1
	offlinePrio = 3

	ipFilterPrio     = 10
	cgroupFilterPrio = 20

	offline_classid = "0x10003"
	Mbit            = 1000 * 1000

	offlineIpset = "caelus-offline"
)

var (
	rootClass    = netlink.MakeHandle(rootMinor, rootMinor)
	onlineClass  = netlink.MakeHandle(rootMinor, onlineMinor)
	offlineClass = netlink.MakeHandle(rootMinor, offlineMinor)
)

// EgressShaper invokes tc command to shape egress network traffic.
// It creates two kinds of filter, tc-cgroup for host network offline pods, tc-ematch (ipset) for global route or
// eni network offline pods. The whole config looks like:

// tc config:
// qdisc htb 1: root refcnt 9 r2q 10 default 2 direct_packets_stat 6
// class htb 1:1 root rate 1500Mbit ceil 1500Mbit burst 189000b cburst 189000b
// class htb 1:2 parent 1:1 prio 1 rate 1500Mbit ceil 1500Mbit burst 189000b cburst 189000b
// class htb 1:3 parent 1:1 prio 3 rate 1000Kbit ceil 1500Mbit burst 1725b cburst 189000b
// filter parent 1: protocol ip pref 10 basic
// filter parent 1: protocol ip pref 10 basic handle 0x1 flowid 1:3
//   ipset(caelus-offline src)
//
// filter parent 1: protocol ip pref 20 cgroup
// filter parent 1: protocol ip pref 20 cgroup handle 0x3

// net_cls config:
// cat /sys/fs/cgroup/net_cls/kubepods/besteffort/podxxx/yyy/net_cls.classid
//65539

// ipset config:
// Name: caelus-offline-eth0
// Type: hash:ip
// Revision: 1
// Header: family inet hashsize 1024 maxelem 65536
// Size in memory: 184
// References: 1
// Members:
// 192.168.2.27
// 192.168.2.28
type EgressShaper struct {
	e            exec.Interface
	ifName       string
	speed        uint64 // weight in Mbit
	ipsetHandle  *ipset.Handle
	offlineIpset string
}

// NewEgressShaper creates an EgressShaper
func NewEgressShaper(ifName string) (*EgressShaper, error) {
	ipsetHandle, err := ipset.New(ipsetLog{})
	if err != nil {
		return nil, err
	}
	return &EgressShaper{
		ifName:       ifName,
		speed:        uint64(machine.GetIfaceSpeed(ifName)),
		ipsetHandle:  ipsetHandle,
		e:            exec.New(),
		offlineIpset: fmt.Sprintf("%s-%s", offlineIpset, ifName),
	}, nil
}

// ReconcileInterface creates htb qdisc and htb class
func (s *EgressShaper) ReconcileInterface() error {
	nic, err := netlink.LinkByName(s.ifName)
	if err != nil {
		return err
	}
	qdiscs, err := netlink.QdiscList(nic)
	if err != nil {
		return err
	}
	linkIndex := nic.Attrs().Index
	htb := netlink.NewHtb(netlink.QdiscAttrs{
		LinkIndex: linkIndex,
		Handle:    netlink.MakeHandle(rootMinor, 0),
		Parent:    netlink.HANDLE_ROOT,
	})
	htb.Defcls = onlineMinor //default class
	var qdiscExpected bool
	if len(qdiscs) == 1 && qdiscs[0].Type() == "htb" {
		existHtb := qdiscs[0].(*netlink.Htb)
		klog.Infof("%s %+v, default: %d", s.ifName, existHtb, existHtb.Defcls)
		if existHtb.Defcls == htb.Defcls && existHtb.Handle == htb.Handle {
			klog.V(5).Infof("%s htb qdisc expected", s.ifName)
			qdiscExpected = true
		} else {
			// delete unexpected htb first
			if err := netlink.QdiscDel(htb); err != nil {
				return fmt.Errorf("%s delete htb qdisc from %v to %v: %v", s.ifName, qdiscs[0], htb, err)
			}
			klog.Infof("%s deleted htb qdisc from %v to %v", s.ifName, qdiscs[0], htb)
		}
	}
	if !qdiscExpected {
		if err := netlink.QdiscAdd(htb); err != nil {
			return fmt.Errorf("%s add htb qdisc: %v", s.ifName, err)
		}
		klog.Infof("%s added htb qdisc %v", s.ifName, htb)
	}
	return s.ensureClasses(nic)
}

func (s *EgressShaper) ensureClasses(nic netlink.Link) error {
	maxCeil := Mbit * s.speed
	linkIndex := nic.Attrs().Index
	if err := s.ensureClass(nic, newClass(linkIndex, netlink.HANDLE_ROOT, rootClass, maxCeil, maxCeil, 0)); err != nil {
		return fmt.Errorf("%s ensure root class: %v", s.ifName, err)
	}
	if err := s.ensureClass(nic, newClass(linkIndex, rootClass, onlineClass, maxCeil, maxCeil, onlinePrio)); err != nil {
		return fmt.Errorf("%s ensure online class: %v", s.ifName, err)
	}
	if err := s.ensureClass(nic, newClass(linkIndex, rootClass, offlineClass, Mbit, maxCeil, offlinePrio)); err != nil {
		return fmt.Errorf("%s ensure offline class: %v", s.ifName, err)
	}
	return nil
}

func (s *EgressShaper) ensureClass(nic netlink.Link, expect *netlink.HtbClass) error {
	classes, err := netlink.ClassList(nic, 0)
	if err != nil {
		return err
	}
	var existing *netlink.HtbClass
	for i := range classes {
		class := classes[i]
		if class.Type() != "htb" {
			klog.Warningf("%s unknown class %v", s.ifName, class)
			continue
		}
		htbClass := class.(*netlink.HtbClass)
		if htbClass.Handle != expect.Handle || htbClass.Parent != expect.Parent {
			continue
		}
		existing = htbClass
		break
	}
	if existing != nil {
		if expect.Rate == existing.Rate && expect.Ceil == existing.Ceil && expect.Prio == existing.Prio &&
			expect.Buffer == existing.Buffer && expect.Cbuffer == existing.Cbuffer {
			return nil
		}
		if err := netlink.ClassChange(expect); err != nil {
			return fmt.Errorf("%s change unexpect htb class from %v to %v: %v", s.ifName, existing, expect, err)
		}
		klog.Infof("%s changed htb class from %v to %v", s.ifName, existing, expect)
		return nil
	}
	if err := netlink.ClassAdd(expect); err != nil {
		return fmt.Errorf("%s create htb class %v: %v", s.ifName, expect, err)
	}
	klog.Infof("%s created htb class: %v", s.ifName, expect)
	return nil
}

func newClass(index int, parent, minor uint32, rate, ceil uint64, prio uint32) *netlink.HtbClass {
	htbClass := netlink.NewHtbClass(netlink.ClassAttrs{
		LinkIndex: index,
		Parent:    parent,
		Handle:    minor,
	}, netlink.HtbClassAttrs{
		Rate: rate,
		Ceil: ceil,
		Prio: prio,
	})
	return htbClass
}

func (s *EgressShaper) ensureCgroupFilter() error {
	// TODO rewrite this when netlink supports basic filter
	data, err := s.e.Command("tc", "filter", "show", "dev", s.ifName).CombinedOutput()
	if err != nil {
		return err
	}
	// expect output
	// filter parent 1: protocol ip pref 10 cgroup
	//filter parent 1: protocol ip pref 10 cgroup handle 0x3
	spec := fmt.Sprintf("handle 0x%s", strconv.FormatInt(offlineMinor, 16))
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.Contains(line, spec) {
			parts := strings.Split(line, " ")
			if len(parts) == 10 {
				return nil
			}
		}
	}
	// tc filter add dev eth1 protocol ip prio 20 parent 1: handle 3: cgroup
	data, err = s.e.Command("tc", "filter", "add", "dev", s.ifName,
		"protocol", "ip", "prio", strconv.Itoa(cgroupFilterPrio),
		"parent", fmt.Sprintf("%d:", rootMinor),
		"handle", fmt.Sprintf("%d:", offlineMinor),
		"cgroup",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s, %v", string(data), err)
	}
	klog.Infof("%s created cgroup filter", s.ifName)
	return nil
}

func (s *EgressShaper) ensureIpSetFilter() error {
	// TODO rewrite this when netlink supports basic filter
	data, err := s.e.Command("tc", "filter", "show", "dev", s.ifName).CombinedOutput()
	if err != nil {
		return err
	}
	ipsetMatch := fmt.Sprintf("ipset(%s src)", s.offlineIpset)
	// expect output
	//filter parent 1: protocol ip pref 20 basic
	//filter parent 1: protocol ip pref 20 basic handle 0x1 flowid 1:10
	//  ipset(offline src)
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, ipsetMatch) {
			return nil
		}
	}
	// tc filter add dev eth1 parent 1: protocol ip prio 10 basic match "ipset(caelus-offline-eth0 src)" flowid 1:3
	args := []string{"filter", "add", "dev", s.ifName,
		"parent", fmt.Sprintf("%d:", rootMinor),
		"protocol", "ip", "prio", strconv.Itoa(ipFilterPrio),
		"basic", "match", ipsetMatch, "flowid", fmt.Sprintf("%d:%d", rootMinor, offlineMinor)}
	data, err = s.e.Command("tc", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create ipset filter, tc %v: %s, %v", args, string(data), err)
	}
	klog.Infof("%s created filter %s", s.ifName, offlineIpset)
	return nil
}

// EnsureEgressOfCgroup ensure egress traffic limit for given cgroup paths
func (s *EgressShaper) EnsureEgressOfCgroup(pathInSubSystems []string) error {
	klog.V(4).Infof("Starting to set network egress control for dev:%s path:%v", s.ifName,
		pathInSubSystems)

	var err error
	if err = s.ensureCgroupFilter(); err != nil {
		return fmt.Errorf("ensure cgroup filter: %v", err)
	}
	// set net_cls cgroup
	root := cgroup.GetRoot()
	for _, pathInSubSystem := range pathInSubSystems {
		err = cgroup.WriteFile([]byte(offline_classid), path.Join(root, "net_cls", pathInSubSystem), "net_cls.classid")
		if err != nil {
			klog.Errorf("set net_cls cgroup %s failed: %v", pathInSubSystem, err)
		}
	}
	return err
}

// EnsureEgressOfIPs ensure egress traffic limit for given ip list
func (s *EgressShaper) EnsureEgressOfIPs(ipStrs []string) error {
	klog.V(4).Infof("Starting to set network egress for dev:%s ip:%s", s.ifName, ipStrs)
	// haship ipset version, set it 1 to make it compatible with old userspace ipset command
	setRevison := uint8(1)
	offlineSet := &ipset.IPSet{
		Name:       s.offlineIpset,
		SetType:    ipset.HashIP,
		Family:     "inet",
		SetRevison: &setRevison,
	}
	if err := s.ipsetHandle.Create(offlineSet); err != nil && err != unix.EEXIST {
		return err
	}
	lists, err := s.ipsetHandle.List(offlineSet.Name)
	if err != nil {
		return err
	}
	if len(lists) == 0 {
		return fmt.Errorf("ipset lists empty")
	}
	existIPs := sets.NewString()
	for _, entry := range lists[0].Entries {
		existIPs.Insert(entry.IP)
	}
	currentIPs := sets.NewString(ipStrs...)

	for _, current := range ipStrs {
		if !existIPs.Has(current) {
			if err := s.ipsetHandle.Add(offlineSet, &ipset.Entry{IP: current}); err != nil {
				klog.Warningf("add %s to offline ipset: %v", current, err)
			}
		}
	}
	for _, exist := range existIPs.UnsortedList() {
		if !currentIPs.Has(exist) {
			if err := s.ipsetHandle.Del(offlineSet, &ipset.Entry{IP: exist}); err != nil {
				klog.Warningf("delete %s from offline ipset: %v", exist, err)
			}
		}
	}
	if err := s.ensureIpSetFilter(); err != nil {
		return fmt.Errorf("ensure ipset filter: %v", err)
	}
	return nil
}
