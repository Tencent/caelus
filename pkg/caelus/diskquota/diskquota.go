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

package diskquota

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tencent/caelus/pkg/caelus/diskquota/manager"
	"github.com/tencent/caelus/pkg/caelus/diskquota/manager/projectquota"
	"github.com/tencent/caelus/pkg/caelus/diskquota/volumes"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"

	"k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type diskQuota struct {
	types.DiskQuotaConfig
	podInformer         cache.SharedIndexInformer
	quotaManager        manager.QuotaManager
	volumeQuotaManagers []volume.VolumeQuotaManager
	handedPods          map[k8stypes.UID]*PodVolumes
	handledLock         sync.RWMutex
}

// NewDiskQuota creates a new disk quota instance
func NewDiskQuota(config types.DiskQuotaConfig, k8s types.K8sConfig,
	podInformer cache.SharedIndexInformer) DiskQuotaInterface {

	// init all kinds of volume manager
	volumeQuotaManagers := []volume.VolumeQuotaManager{
		volume.NewRootFsDiskQuota(config.ContainerRuntime, config.VolumeSizes[types.VolumeTypeRootFs]),
		volume.NewEmptyDirQuota(config.VolumeSizes[types.VolumeTypeEmptyDir], k8s.KubeletRootDir),
		volume.NewHostPathQuota(config.VolumeSizes[types.VolumeTypeHostPath]),
	}
	return &diskQuota{
		DiskQuotaConfig:     config,
		podInformer:         podInformer,
		quotaManager:        projectquota.NewProjectQuota(),
		volumeQuotaManagers: volumeQuotaManagers,
		handedPods:          make(map[k8stypes.UID]*PodVolumes),
	}
}

// module name
func (d *diskQuota) Name() string {
	return "ModuleDiskQuota"
}

// Run enters main loop
func (d *diskQuota) Run(stop <-chan struct{}) {
	var lastUpdate time.Time
	go wait.Until(func() {
		// assign quota
		lastUpdate = time.Now()
		err := d.handleDiskQuota()
		if err == nil {
			// remove pods who has not update last time
			d.removeExitedPods(lastUpdate)
		}
		if klog.V(3).Enabled() {
			d.printVolumes()
		}

		// clear quota for paths which not existed
		d.clearDiskQuota()
	}, d.CheckPeriod.TimeDuration(), stop)

	klog.V(2).Infof("disk quota running successfully")
}

// GetPodDiskQuota return all volumes of the pod
func (d *diskQuota) GetPodDiskQuota(pod *v1.Pod) (map[types.VolumeType]*VolumeInfo, error) {
	d.handledLock.RLock()
	defer d.handledLock.RUnlock()
	podVolumes, ok := d.handedPods[pod.UID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return podVolumes.Volumes, nil
}

// GetAllPodsDiskQuota return all volumes for all the pods on the node
func (d *diskQuota) GetAllPodsDiskQuota() ([]*PodVolumes, error) {
	var podVolumes []*PodVolumes
	d.handledLock.RLock()
	defer d.handledLock.RUnlock()

	for _, podVolume := range d.handedPods {
		podVolumes = append(podVolumes, podVolume)
	}

	return podVolumes, nil
}

func (d *diskQuota) handleDiskQuota() error {
	podList := d.podInformer.GetStore().List()

	d.handledLock.Lock()
	defer d.handledLock.Unlock()

	for _, p := range podList {
		pod := p.(*v1.Pod)
		// if pod terminated, just ignore
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		d.handlePodDiskQuota(pod)
	}

	return nil
}

// handlePodDiskQuota will get newest quota usage if quota has been set, and set quota if not set.
// it must be called with holding handledLock
func (d *diskQuota) handlePodDiskQuota(pod *v1.Pod) {
	podVolume, ok := d.handedPods[pod.UID]
	if !ok {
		podVolume = &PodVolumes{
			Pod:      pod,
			AppClass: appclass.GetAppClass(pod),
			Volumes:  make(map[types.VolumeType]*VolumeInfo),
		}
		// store result
		d.handedPods[pod.UID] = podVolume
	}
	// must update timestamp, no matter success or failed
	podVolume.lastTime = time.Now()

	// get quota path from all volume types
	for _, v := range d.volumeQuotaManagers {
		vinfo, ok := podVolume.Volumes[v.Name()]
		if !ok {
			vinfo = &VolumeInfo{
				getPathsSuccess:  false,
				setQuotasSuccess: false,
				Paths:            make(map[string]*PathInfoWrapper),
			}
			podVolume.Volumes[v.Name()] = vinfo
		}

		// no need to set again if already set quota successfully
		// there is one case no covered, which pod has changed the quota size in annotation.
		if vinfo.setQuotasSuccess {
			// update quota size
			d.getVolumeQuotaSize(vinfo)
			continue
		}

		if !vinfo.getPathsSuccess {
			err := d.getPathsInfo(vinfo, v, pod)
			if err != nil {
				klog.Errorf("get %s paths info err for pod: %s-%s", v.Name(), pod.Namespace, pod.Name)
			}
		}
	}

	// set quota for each path
	for _, v := range d.volumeQuotaManagers {
		vinfo := podVolume.Volumes[v.Name()]
		// do not set quota either get path failed or already set quota successfully
		if !vinfo.getPathsSuccess || vinfo.setQuotasSuccess {
			continue
		}

		vinfo.setQuotasSuccess = true
		for _, p := range vinfo.Paths {
			if p.setQuotaSuccess {
				// update quota size
				d.getPathQuotaSize(p)
				continue
			}

			err := d.setPathQuota(p, v)
			if err != nil {
				klog.Errorf("set pod(%s-%s) volume(%s) disk quota for path %s err: %v",
					pod.Namespace, pod.Name, v.Name(), p.Path, err)
				vinfo.setQuotasSuccess = false
			} else {
				klog.V(4).Infof("set pod(%s-%s) volume(%s) disk quota for path %s success",
					pod.Namespace, pod.Name, v.Name(), p.Path)
			}
		}
	}
}

// removeExitedPods remove pods, who has not update before lastUpdate timestamp
func (d *diskQuota) removeExitedPods(lastUpdate time.Time) {
	d.handledLock.Lock()
	defer d.handledLock.Unlock()

	for uid, volumes := range d.handedPods {
		if volumes.lastTime.Before(lastUpdate) {
			klog.Warningf("deleting volume record for pod: %s-%s", volumes.Pod.Namespace, volumes.Pod.Name)
			delete(d.handedPods, uid)
		}
	}
}

// clearDiskQuota clear quota path which has not existed.
// for we do not known when the pod is gone without watch, just check the paths which are gone.
func (d *diskQuota) clearDiskQuota() {
	existingPaths := d.quotaManager.GetAllQuotaPath()
	klog.V(3).Infof("existing quota paths: %+v", existingPaths)

	d.handledLock.RLock()
	defer d.handledLock.RUnlock()
	currentPaths := sets.String{}
	for _, volumes := range d.handedPods {
		for _, vinfo := range volumes.Volumes {
			for _, p := range vinfo.Paths {
				currentPaths.Insert(p.Path)
			}
		}
	}

	for vType, paths := range existingPaths {
		switch vType {
		case types.VolumeTypeHostPath:
			// for host path, check if the path has already been deleted from cache, and deleting quota if not existed
			for _, path := range paths.List() {
				if currentPaths.Has(path) {
					continue
				}

				klog.V(2).Infof("hot path(%s) has not found, starting cleaning disk quota", path)
				err := d.quotaManager.ClearQuota(path)
				if err != nil {
					klog.Errorf("clear disk quota for path %s err: %v", path, err)
				}
			}
		default:
			// for other paths, check if the path is still existing, and deleting quota if not
			for _, path := range paths.List() {
				_, err := os.Stat(path)
				if err == nil {
					continue
				}
				if err != nil && !os.IsNotExist(err) {
					klog.Errorf("check path(%s) err: %v", path, err)
					continue
				}

				klog.V(2).Infof("path(%s) not found, starting cleaning disk quota", path)
				err = d.quotaManager.ClearQuota(path)
				if err != nil {
					klog.Errorf("clear disk quota for path %s err: %v", path, err)
				}
			}
		}
	}
}

func (d *diskQuota) printVolumes() {
	d.handledLock.RLock()
	defer d.handledLock.RUnlock()

	formatStr := "\ncurrent pod volume paths:\n"
	for _, podVolumes := range d.handedPods {
		formatStr += fmt.Sprintf("-- namespace:%s, pod:%s, lastTime: %s\n",
			podVolumes.Pod.Namespace, podVolumes.Pod.Name, podVolumes.lastTime.Format("2006-01-02 15:04:05"))
		for vType, vol := range podVolumes.Volumes {
			if len(vol.Paths) == 0 {
				continue
			}
			formatStr += fmt.Sprintf("     %s with path %v, and set quota %+v, the paths are:\n",
				vType, vol.getPathsSuccess, vol.setQuotasSuccess)
			for _, p := range vol.Paths {
				formatStr += fmt.Sprintf("       %s\n", p.Path)

			}
		}
	}
	klog.Info(formatStr)
}

func (d *diskQuota) getPathsInfo(vInfo *VolumeInfo, vQuota volume.VolumeQuotaManager, pod *v1.Pod) error {
	vInfo.getPathsSuccess = true
	qPaths, err := vQuota.GetVolumes(pod)
	if err != nil {
		vInfo.getPathsSuccess = false
		return err
	}

	for name, p := range qPaths {
		vInfo.Paths[name] = &PathInfoWrapper{
			setQuotaSuccess: false,
			PathInfo:        *p,
		}
	}
	return nil
}

func (d *diskQuota) setPathQuota(p *PathInfoWrapper, vQuota volume.VolumeQuotaManager) error {
	p.setQuotaSuccess = true
	err := d.quotaManager.SetQuota(p.Path, vQuota.Name(), p.Size, p.SharedInfo)
	if err != nil {
		// if quota not supported, just treat it as success
		if !manager.IsNotSupported(err) {
			p.setQuotaSuccess = false
			return err
		}
	}

	return nil
}

func (d *diskQuota) getPathQuotaSize(p *PathInfoWrapper) {
	size, err := d.quotaManager.GetQuota(p.Path)
	if err != nil {
		if !manager.IsNotSupported(err) {
			klog.Errorf("get disk quota for %s err: %v", p.Path, err)
		}
	}
	p.Size = size
}

func (d *diskQuota) getVolumeQuotaSize(v *VolumeInfo) {
	for _, p := range v.Paths {
		d.getPathQuotaSize(p)
	}
}
