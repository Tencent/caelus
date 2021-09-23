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

### build

``` sh
# binary build, which generates binary under _output/bin/
$ make build

# image build
$ make image

# run unit test
$ make test
```

### Run

```sh
# running in script
$ caelus --config=hack/config/caelus.json --hostname-override=xxx --v=2

# running in image
$ kubectl create -f hack/yaml/caelus.json
$ kubectl label node colation=true
$ kubectl -n kube-system get daemonset
```

## Contributing
For more information about contributing issues or pull requests, see our [Contributing to Caelus](doc/contributing.md).

## License
Caelus is under the Apache License 2.0. See the [License](LICENSE) file for details.
