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

package hadoop

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	global "github.com/tencent/caelus/pkg/types"

	"k8s.io/klog"
)

const (
	// YarnSiteFile show nodemanager special config file
	YarnSiteFile = "yarn-site.xml"
	// HdfsSiteFILE show hdfs special config file
	HdfsSiteFILE = "hdfs-site.xml"
	// CoreSiteFile show nodemanager special config file
	CoreSiteFile = "core-site.xml"
	// YarnEnvFile show nodemanager special config file
	YarnEnvFile = "yarn-env.sh"

	// the NodeManager has minimum requirement
	minCapacityCores    = "1"
	minCapacityMemoryMB = "1024"
)

var (
	resourcemanagerAddress = ""
	nodemanagerAddress     = ""
	nodemanagerWebAddress  = ""

	// NodeManager capacity
	cacheCapacity     *global.NMCapacity
	cacheCapacityLock sync.RWMutex
)

// PropertyData show value of signal xml property
type PropertyData struct {
	XMLName     xml.Name `xml:"property"`
	Name        string   `xml:"name"`
	Value       string   `xml:"value"`
	Description string   `xml:"description,omitempty"`
}

// ConfData show whole xml properties
type ConfData struct {
	XMLName    xml.Name       `xml:"configuration"`
	Properties []PropertyData `xml:"property"`
}

// GetXMLFullPath return full path
func GetXMLFullPath(filename string) string {
	confDir := os.Getenv("HADOOP_CONF_DIR")
	return fmt.Sprintf("%s/%s", confDir, filename)
}

// LoadConfDataFromStream load xml struct from io stream
func LoadConfDataFromStream(s io.Reader) (*ConfData, error) {
	conf := &ConfData{}
	data, err := ioutil.ReadAll(s)
	if err != nil {
		return conf, err
	}

	err = xml.Unmarshal(data, conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

// Get function get value based on key
func (conf *ConfData) Get(key string) string {
	for _, prop := range conf.Properties {
		if prop.Name == key {
			return prop.Value
		}
	}
	return ""
}

// Set function set value based on key
func (conf *ConfData) Set(key string, value string) {
	for i, prop := range conf.Properties {
		if prop.Name == key {
			conf.Properties[i].Value = value
			// should not return, same key may existed
		}
	}
}

// SetAdd function set value based on key, including key not exited
func (conf *ConfData) SetAdd(key string, value string) {
	find := false
	for i, prop := range conf.Properties {
		if prop.Name == key {
			conf.Properties[i].Value = value
			find = true
		}
	}
	if !find {
		property := PropertyData{
			Name:  key,
			Value: value,
		}
		conf.Properties = append(conf.Properties, property)
	}
}

// SaveToStream restore io stream to file
func (conf *ConfData) SaveToStream(w io.Writer) error {
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "    ")
	return encoder.Encode(conf)
}

// SetMultipleConfDataToFile set many properties to xml file
func SetMultipleConfDataToFile(xmlfile string, properties map[string]string) error {
	if len(properties) == 0 {
		return nil
	}

	file, err := os.OpenFile(GetXMLFullPath(xmlfile), os.O_RDWR, 0666)
	if err != nil {
		klog.Errorf("open file error: %v", err)
		return err
	}
	defer file.Close()

	conf, err := LoadConfDataFromStream(file)
	if err != nil {
		return err
	}

	for key, value := range properties {
		conf.Set(key, value)
	}

	file.Seek(0, os.SEEK_SET)
	file.Truncate(0)
	return conf.SaveToStream(file)
}

// SetAddDelMultipleConfDataToFile set many properties to xml file, may add new key or delete old key
func SetAddDelMultipleConfDataToFile(xmlfile string, properties map[string]string, add, del bool) error {
	if len(properties) == 0 {
		return nil
	}

	file, err := os.OpenFile(GetXMLFullPath(xmlfile), os.O_RDWR, 0666)
	if err != nil {
		klog.Errorf("open file error: %v", err)
		return err
	}
	defer file.Close()

	conf, err := LoadConfDataFromStream(file)
	if err != nil {
		return err
	}

	if add {
		for key, value := range properties {
			conf.SetAdd(key, value)
		}
	}

	if del {
		newProperties := []PropertyData{}
		for _, p := range conf.Properties {
			if _, ok := properties[p.Name]; ok {
				newProperties = append(newProperties, p)
			}
		}
		conf.Properties = newProperties
	}

	file.Seek(0, os.SEEK_SET)
	file.Truncate(0)
	return conf.SaveToStream(file)
}

// LoadConfDataFromFile load xmf struct from file
func LoadConfDataFromFile(filename string) (*ConfData, error) {
	file, err := os.Open(GetXMLFullPath(filename))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	conf, err := LoadConfDataFromStream(file)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// GetConfDataFromFile get key value from file
// @xmlfile: yarn-site.xml, core-site.xml, hdfs-site.xml
func GetConfDataFromFile(xmlfile string, key string) string {
	file, err := os.Open(GetXMLFullPath(xmlfile))
	if err != nil {
		return ""
	}
	defer file.Close()

	conf, err := LoadConfDataFromStream(file)
	if err != nil {
		return ""
	}

	return conf.Get(key)
}

// GetCapacity get capacity value
func GetCapacity() (global.NMCapacity, error) {
	capacity := global.NMCapacity{}

	// get NodeManager capacity from cache firstly
	ok := func() bool {
		cacheCapacityLock.RLock()
		defer cacheCapacityLock.RUnlock()
		if cacheCapacity != nil {
			capacity.Vcores = cacheCapacity.Vcores
			capacity.MemoryMB = cacheCapacity.MemoryMB
			return true
		}
		return false
	}()
	if ok {
		return capacity, nil
	}

	// get NodeManager capacity from file
	conf, err := LoadConfDataFromFile(YarnSiteFile)
	if err != nil {
		return capacity, err
	}

	memoryStr := conf.Get("yarn.nodemanager.resource.memory-mb")
	vcoresStr := conf.Get("yarn.nodemanager.resource.cpu-vcores")
	memMB, err := strconv.Atoi(memoryStr)
	if err != nil {
		klog.Errorf("read capacity memory err, set -1: %v", err)
		capacity.MemoryMB = -1
	} else {
		capacity.MemoryMB = int64(memMB)
	}
	vcores, err := strconv.Atoi(vcoresStr)
	if err != nil {
		klog.Errorf("read capacity cores err, set -1: %v", err)
		capacity.Vcores = -1
	} else {
		capacity.Vcores = int64(vcores)
	}

	return capacity, nil
}

// SetCapacity set capacity value
func SetCapacity(capacity global.NMCapacity) error {
	func() {
		cacheCapacityLock.Lock()
		cacheCapacityLock.Unlock()

		if cacheCapacity == nil {
			cacheCapacity = &global.NMCapacity{}
		}
		cacheCapacity.Vcores = capacity.Vcores
		cacheCapacity.MemoryMB = capacity.MemoryMB
	}()

	// save to file, in case nodemanager process restarted
	properties := map[string]string{
		"yarn.nodemanager.resource.memory-mb":  strconv.Itoa(int(capacity.MemoryMB)),
		"yarn.nodemanager.resource.cpu-vcores": strconv.Itoa(int(capacity.Vcores)),
	}
	// the nodemanager process will start failed with zero value
	if capacity.MemoryMB == 0 {
		klog.Errorf("zero value when writing memory capacity, just set default value: %v", minCapacityMemoryMB)
		properties["yarn.nodemanager.resource.memory-mb"] = minCapacityMemoryMB
	}
	if capacity.Vcores == 0 {
		klog.Errorf("zero value when writing cpu capacity, just set default value: %v", minCapacityCores)
		properties["yarn.nodemanager.resource.cpu-vcores"] = minCapacityCores
	}

	return SetMultipleConfDataToFile(YarnSiteFile, properties)
}

// GetConfig get property based on keys
func GetConfig(fileName string, keys []string) (map[string]string, error) {
	conf, err := LoadConfDataFromFile(fileName)
	if err != nil {
		return map[string]string{}, err
	}

	property := make(map[string]string)
	for _, key := range keys {
		property[key] = conf.Get(key)
	}
	return property, nil
}

// GetAllConfig get all properties from file
func GetAllConfig(fileName string) (map[string]string, error) {
	conf, err := LoadConfDataFromFile(fileName)
	if err != nil {
		return map[string]string{}, err
	}

	property := make(map[string]string)
	for _, prop := range conf.Properties {
		property[prop.Name] = prop.Value
	}

	return property, nil
}

// SetConfig set properties for file
func SetConfig(fileName string, properties map[string]string) error {
	return SetMultipleConfDataToFile(fileName, properties)
}

// SetAddDelConfig set properties for file, may add new or delete old
func SetAddDelConfig(fileName string, properties map[string]string, add, del bool) error {
	return SetAddDelMultipleConfDataToFile(fileName, properties, add, del)
}

// handle property such as:
//   <property>
//        <name>yarn.resourcemanager.webapp.address.rm1</name>
//        <value>${yarn.resourcemanager.hostname.rm1}:8080</value>
//    </property>
var propertyRegex, _ = regexp.Compile(`\${(.*)}:(.*)`)

// getRealResourceManagerAddress get resource manager address
func getRealResourceManagerAddress(rmAddr string) string {
	if !strings.Contains(rmAddr, "${") {
		return rmAddr
	}
	matches := propertyRegex.FindStringSubmatch(rmAddr)
	if len(matches) != 3 {
		return ""
	}

	return GetConfDataFromFile(YarnSiteFile, matches[1]) + ":" + matches[2]
}

// GetResourceManagerAddress get resource manager address
// - cacheAddress: get result from cache, no need to read file
func GetResourceManagerAddress(cacheAddress bool) string {
	if cacheAddress && len(resourcemanagerAddress) != 0 {
		return resourcemanagerAddress
	}

	rmAddr := GetConfDataFromFile(YarnSiteFile, "yarn.resourcemanager.webapp.address")
	if len(rmAddr) != 0 {
		resourcemanagerAddress = getRealResourceManagerAddress(rmAddr)
	} else {
		rm1Addr := GetConfDataFromFile(YarnSiteFile, "yarn.resourcemanager.webapp.address.rm1")
		rm1Addr = getRealResourceManagerAddress(rm1Addr)
		if rm1Addr == "" {
			klog.Error("get nil address for webapp.address.rm1")
			return ""
		}
		rm2Addr := GetConfDataFromFile(YarnSiteFile, "yarn.resourcemanager.webapp.address.rm2")
		rm2Addr = getRealResourceManagerAddress(rm2Addr)
		if rm2Addr == "" {
			klog.Error("get nil address for webapp.address.rm2")
			return ""
		}
		resourcemanagerAddress = rm1Addr + "," + rm2Addr
	}

	klog.Infof("resourcemanager address:%s\n", resourcemanagerAddress)
	return resourcemanagerAddress
}

// GetNodeManagerAddress get nodemanager address
// - cacheAddress: get result from cache, no need to read file
func GetNodeManagerAddress(cacheAddress bool) string {
	if cacheAddress && len(nodemanagerAddress) != 0 {
		return nodemanagerAddress
	}

	nodemanagerAddress = GetConfDataFromFile(YarnSiteFile, "yarn.nodemanager.address")
	klog.Infof("nodemanager address:%s\n", nodemanagerAddress)
	return nodemanagerAddress
}

// GetNodeManagerWebAddress get nodemanager webapp address
// - cacheAddress: get result from cache, no need to read file
func GetNodeManagerWebAddress(cacheAddress bool) string {
	if cacheAddress && len(nodemanagerWebAddress) != 0 {
		return nodemanagerWebAddress
	}

	nodemanagerWebAddress = GetConfDataFromFile(YarnSiteFile, "yarn.nodemanager.webapp.address")
	klog.Infof("nodemanager web address:%s\n", nodemanagerWebAddress)
	return nodemanagerWebAddress
}
