# nm_operator

## description

  nm_operator is a http server, which packaging YARN execution commands. By calling API, users could execute YARN command
remotely, such as starting NodeManager, or updating configuration file. Now the supported API are as following:
 - /v1/nm/status,  check if NodeManager is running
 - /v1/nm/capacity, get or update NodeManager resource capacity, including vcores and memory
 - /v1/nm/property, get or update configuration file, such as yarn-site.xml
 - /v1/nm/start, start NodeManager process
 - /v1/nm/stop, stop NodeManager process
 - /v1/nm/schedule/disable, disable NodeManager accepting new jobs
 - /v1/nm/schedule/enable, recover NodeManager accepting new jobs
 - /v1/nm/capacity/update, notify ResourceManager to update resource

## How to use
  Users prepare a base image which including the NodeManager process, and copy the nm_operator binary into Dockerfile as the 
ENTRYPOINT or CMD to build a new image. Then users could submit a workload on kubernetes with the new image, the workload
must be scheduled on the nodes where caelus is running, let caelus call the API to execute YARN commands. Uses could follow
the example [Yaml file](../hack/yaml/nodemanager.yaml).