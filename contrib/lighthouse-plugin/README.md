# Lighthouse Plugin

This repository implements a bundle of plugin to enhance the Kubernetes function. The provided features are
listed below.


- Docker runtime backend

*1.* Storage Options

Any annotations start with `mixer.kubernetes.io/storage-opt-` will be added to docker storage options.

```
apiVersion: v1
kind: Pod
metadata:
  ...
  annotations:
    mixer.kubernetes.io/storage-opt-size: "1G"
```

This above example means add `size=1G` to your container.

*2.* uts Options

Any annotations start with `mixer.kubernetes.io/uts-mode` will not use host namespace when host network is true.

```
apiVersion: v1
kind: Pod
metadata:
  ...
  annotations:
    mixer.kubernetes.io/uts-mode: ""
```

*3.* Pid limit

To prevent user's pids flood the host machine, we provide a method to set pid limit for user's container. If you want this feature,
set `mixer.kubernetes.io/pids-limit: <limit>` to your pod's annotations.

```
apiVersion: v1
kind: Pod
metadata:
  ...
  annotations:
    mixer.kubernetes.io/pids-limit: "40"
spec:
  containers:
  - name: test
```

*4.* Offline Options

Offline pods with annotation `mixer.kubernetes.io/app-class:greedy` will be changed the cgroup path to "kubepods/offline".

```
apiVersion: v1
kind: Pod
metadata:
  ...
  annotations:
    mixer.kubernetes.io/app-class: "greedy"
```
