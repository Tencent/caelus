/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

This file is copied from https://github.com/kubernetes/kubernetes/pkg/kubelet/eviction/threshold_notifier_linux.go
*/

package notify

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

const (
	// eventSize is the number of bytes returned by a successful read from an eventfd
	// see http://man7.org/linux/man-pages/man2/eventfd.2.html for more information
	eventSize = 8
	// numFdEvents is the number of events we can record at once.
	// If EpollWait finds more than this, they will be missed.
	numFdEvents = 6
	// this prevents constantly updating the memcg notifier if synchronize
	// is run frequently.
	notifierRefreshInterval = 10 * time.Second
)

// linuxCgroupNotify describe cgroup notify data
type linuxCgroupNotify struct {
	eventfd  int
	epfd     int
	stopCh   chan struct{}
	stopLock sync.Mutex
}

// newCgroupNotify returns a linuxCgroupNotifier, which performs cgroup control operations required
// to receive notifications from the cgroup.
func newCgroupNotify(path, attribute string, notifyStr string) (*linuxCgroupNotify, error) {
	var watchfd, eventfd, epfd, controlfd int
	var err error
	watchfd, err = unix.Open(fmt.Sprintf("%s/%s", path, attribute), unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(watchfd)
	controlfd, err = unix.Open(fmt.Sprintf("%s/cgroup.event_control", path), unix.O_WRONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(controlfd)
	eventfd, err = unix.Eventfd(0, unix.EFD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if eventfd < 0 {
		err = fmt.Errorf("eventfd call failed")
		return nil, err
	}
	defer func() {
		// Close eventfd if we get an error later in initialization
		if err != nil {
			unix.Close(eventfd)
		}
	}()
	epfd, err = unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if epfd < 0 {
		err = fmt.Errorf("EpollCreate1 call failed")
		return nil, err
	}
	defer func() {
		// Close epfd if we get an error later in initialization
		if err != nil {
			unix.Close(epfd)
		}
	}()
	config := fmt.Sprintf("%d %d %s", eventfd, watchfd, notifyStr)
	_, err = unix.Write(controlfd, []byte(config))
	if err != nil {
		return nil, err
	}
	return &linuxCgroupNotify{
		eventfd: eventfd,
		epfd:    epfd,
		stopCh:  make(chan struct{}),
	}, nil
}

// start function waits event
func (n *linuxCgroupNotify) start(eventCh chan<- struct{}) {
	err := unix.EpollCtl(n.epfd, unix.EPOLL_CTL_ADD, n.eventfd, &unix.EpollEvent{
		Fd:     int32(n.eventfd),
		Events: unix.EPOLLIN,
	})
	if err != nil {
		klog.Warningf("eviction manager: error adding epoll eventfd: %v", err)
		return
	}
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}
		event, err := wait(n.epfd, n.eventfd, notifierRefreshInterval)
		if err != nil {
			klog.Warningf("eviction manager: error while waiting for memcg events: %v", err)
			return
		} else if !event {
			// Timeout on wait.  This is expected if the threshold was not crossed
			continue
		}
		// Consume the event from the eventfd
		buf := make([]byte, eventSize)
		_, err = unix.Read(n.eventfd, buf)
		if err != nil {
			klog.Warningf("eviction manager: error reading memcg events: %v", err)
			return
		}
		eventCh <- struct{}{}
	}
}

// wait waits up to notifierRefreshInterval for an event on the Epoll FD for the
// eventfd we are concerned about.  It returns an error if one occurs, and true
// if the consumer should read from the eventfd.
func wait(epfd, eventfd int, timeout time.Duration) (bool, error) {
	events := make([]unix.EpollEvent, numFdEvents+1)
	timeoutMS := int(timeout / time.Millisecond)
	n, err := unix.EpollWait(epfd, events, timeoutMS)
	if n == -1 {
		if err == unix.EINTR {
			// Interrupt, ignore the error
			return false, nil
		}
		return false, err
	}
	if n == 0 {
		// Timeout
		return false, nil
	}
	if n > numFdEvents {
		return false, fmt.Errorf("epoll_wait returned more events than we know what to do with")
	}
	for _, event := range events[:n] {
		if event.Fd == int32(eventfd) {
			if event.Events&unix.EPOLLHUP != 0 || event.Events&unix.EPOLLERR != 0 || event.Events&unix.EPOLLIN != 0 {
				// EPOLLHUP: should not happen, but if it does, treat it as a wakeup.

				// EPOLLERR: If an error is waiting on the file descriptor, we should pretend
				// something is ready to read, and let unix.Read pick up the error.

				// EPOLLIN: There is data to read.
				return true, nil
			}
		}
	}
	// An event occurred that we don't care about.
	return false, nil
}

// stop function closes event fd
func (n *linuxCgroupNotify) stop() {
	n.stopLock.Lock()
	defer n.stopLock.Unlock()
	select {
	case <-n.stopCh:
		// the linuxCgroupNotifier is already stopped
		return
	default:
	}
	unix.Close(n.eventfd)
	unix.Close(n.epfd)
	close(n.stopCh)
}
