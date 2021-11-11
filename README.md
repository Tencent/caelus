## Caelus
  [![GitHub license](https://img.shields.io/badge/license-Apache--2.0-brightgreen)](https://github.com/Tencent/caelus/blob/master/LICENSE)
  [![Release](https://img.shields.io/github/v/release/Tencent/caelus.svg)](https://github.com/Tencent/caelus/releases)
  [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/Tencent/caelus/pulls)

  Caelus is a set of Kubernetes solutions for reusing idle resources of nodes by running extra batch jobs, these resources come from 
  the underutilization of online jobs, especially during low traffic periods. To make batch jobs compatible with online jobs,
  caelus dynamically manages multiple resource isolation mechanisms and also checks abnormalities of various metrics. 
  Batch jobs will be throttled or even killed if interference detected.

## Features

* Collect various metrics, including node resources, cgroup resources and online jobs latency

* Batch jobs could be running on YARN or Kubernetes

* Predict total resource usages of the node, including online jobs and kernel modules, such as slab

* Dynamically manage multiple resource isolation mechanisms, such as CPU, memory, and disk space

* Dynamically check abnormalities of various metrics, such as CPU usage or online jobs latency

* Throttle or even kill batch jobs when resource pressure or latency spike detected

* Prometheus metrics supported

* Alarm supported

## Usage

Find more usage at [Tutorial.md](doc/tutorial.md). The project also have two attached tools:

#### nm_operator

[nm_operator](doc/nm_operator.md) is used to execute YARN commands in the way of remote API.

#### metric_adapter

[metric_adapter](doc/metric_adapter.md) is used to collect more application metrics with adapter extension.


## Getting started

### Build

``` sh
# binary build, which generates binary under _output/bin/
$ make build

# image build
$ make image

# run unit test
$ make test
```

### Run
The caelus should better run on the node with the kubelet process, and write the kubelet's "root-dir" value to "kubelet_root_dir" in [config file](hack/config/caelus.json).

The [config file](hack/config/caelus.json) and [rule file](hack/config/rules.json) are just the example files, you could add more
feature based on different demands.

```sh
# running in script
$ mkdir -p /etc/caelus/
$ cp hack/config/rules.json /etc/caelus/
$ # if the batch job is running on YARN, you must modify the "offline_type" in hack/config/caelus.json as "yarn_on_k8s", and run the command
$ caelus --config=hack/config/caelus.json --v=2
$ # if the batch job is running on K8S, you must modify the "offline_type" in hack/config/caelus.json as "k8s", and run the command
$ # if the command is running inside the pod, then you could ignore the kubeconfig parameter
$ caelus --config=hack/config/caelus.json --hostname-override=xxx --v=2 --kubeconfig=xxx

# run in container
$ # the container parameters and environments could be found from hack/yaml/caelus.json, such as:
$ docker run -it --cap-add SYS_ADMIN --cap-add NET_ADMIN --cap-add MKNOD --cap-add SYS_PTRACE --cap-add SYS_CHROOT --cap-add SYS_NICE -v /:/rootfs -v /sys:/sys -v /dev/disk:/dev/disk ccr.ccs.tencentyun.com/caelus/caelus:v1.0.0 /bin/bash

# running on K8S
$ kubectl create -f hack/yaml/caelus.yaml
$ kubectl label node colation=true
$ kubectl -n kube-system get daemonset
```

## Contributing
For more information about contributing issues or pull requests, see our [Contributing to Caelus](doc/contributing.md).

## License
Caelus is under the Apache License 2.0. See the [License](LICENSE) file for details.
