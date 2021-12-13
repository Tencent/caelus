# 配置文件说明[caelus.json](../hack/config/caelus.json)

### 1、k8s_config
k8s的相关参数配置

（1）kubelet_root_dir： kubelet的root-dir参数

### 2、check_point
caelus进程重启恢复状态

（1）check_point_dir：checkpoint的内容存放目录。若caelus运行在容器中，此目录需为hostpath

### 3、task_type
配置在线和离线作业类型

（1）online_type：local、k8s

（2）offline_type：k8s、 yarn_on_k8s

### 4、node_resource
实时通知离线master（K8s或YARN）该节点的离线可用资源

（1）模块功能可关闭

（2）disable_kill_if_normal：若在线资源使用上升导致离线可用资源降低，但此时节点整体资源使用正常，此时可以不对离线作业驱逐，
即使离线作业的资源申请量或使用量超出可用资源量。

（3）yarn_config：针对离线作业通过YARN提交的场景

- capacity_inc_interval：nodemanager可用资源恢复（离线资源刚压制结束，开始恢复）过程中，控制步长，慢点恢复，不要过于频繁调整
- nm_server：nm_operator地址
- resource_roundoff：对离线资源进行规整化，如内存配置为512的倍数，避免频繁调整
- resource_range：划定离线资源波动范围。在范围内波动的资源变化，认为波动比较少，不需要更新到master端，避免频繁调整。
- port_auto_detect： nodemanager端口自动检测，混部机器上nodemaanger默认的端口可能已被占用
- properties：增加或修改yarn-site.xml中的配置，可通过caelus直接完成。如："yarn.nodemanager.delete.debug-delay-sec": "180"
- disks：磁盘检测功能。根据混部机器上磁盘空间和ratio_to_core来计算离线可用资源，避免接收过多的离线作业，导致容易出现磁盘满问题
- cpu_over_commit：离线资源超发。解决分配率高，但使用率低问题，可对不同的时段配置不同的超发比例 

（4）silence：配置离线禁止运行时段。如离线作业不能在白天运行，只能在晚上0：00-6：00之间运行。
ahead_of_unSchedule表示提前多长时间禁止接收离线作业，让已运行的离线作业优雅退出

### 5、predicts
在线资源预测，目前支持cpu和内存

（1）若关闭改模块，则在线预测数据输出为0

（2）predict_type：支持"local"和"vpa"，其中local使用caelus内置的预测算法，vpa表示从远程获取在线预测结果（名字有些误导，remote更好些）

（3）predict_server_addr：若选择远程预测，即vpa模式，填写远程server地址。其server必须支持api get请求：http://xx/v1/predict/online/{node}，
其返回格式为： {"cpu":"1000m", "memory":"1024Mi"}

（4）reserve_resource：在线资源buffer。离线可用资源=机器总资源 - 在线预测资源 - buffer资源，避免在线突增引发干扰问题。
若关闭预测，则离线可用资源即为去除buffer资源后的剩余资源。根据此特性可灵活扩展到不同的场景。

### 6、metrics
指标收集模块，收集节点资源、cgroup资源、perf和RDT指标，及自定义指标

（1）node：节点资源收集，包括cpu、内存、磁盘io、网络io、进程状态

- 可配置节点资源收集的时间间隔
- ifaces_with_property：区分主网卡，还是浮动ip网卡（pod独立网卡）。若是正常网卡填写如eth1_normal, 若是浮动ip网卡，则填写如eth2_eni。
- disk_names：磁盘名称列表

（2）container：cadvisor收集容器（cgroup）级别指标

- resources：收集哪些资源，包括cpu，memory等cadvisor支持的指标，可查看github cadvisor/container/factory.go中列出的资源名称
- cgroup：收集哪些cgroup目录及子目录，避免扫描其他无用的cgroup，浪费性能开销

（3）perf：通过perf命令收集cgroup级别指标，包括cpu指令数、cpu指令执行周期数、CPI、cache miss、LLC cache。

- collect_interval：多长时间收集一次
- collect_duration：每次收集持续时间
- ignored_cgroups：跳过不需要收集指标的cgroup目录

（4）rdt：通过intel RDT工具收集的指标
（工具详见：https://www.intel.cn/content/www/cn/zh/architecture-and-technology/resource-director-technology.html ），
包括cache miss、LLC、内存带宽MRL和MBR等

- rdt_command： rdt工具路径
- collect_interval：每隔多长时间收集一次
- collect_duration：RDT工具运行过程中，收集数据的持续时间，即参数-t
- execute_interval：RDT工具运行过程中，收集数据的间隔时间，即参数-i

（5）prometheus：兼容节点其他组件输出的prometheus指标，如node_exporter输出的系统故障指标。
另外用户可以在/var/run/caelus/keys文件中增加固定的prometheus指标，通过caelus的metrics接口输出。

- disable_show：外部的prometheus是否通过caelus的metrics接口输出
- items：配置外部的prometheus地址，同时可以配置只收集哪些指标，或忽略哪些指标

### 7、resource_isolate
节点离线资源隔离模块，对离线资源进行限制

- 用户可关闭离线资源隔离模块功能
- resource_disable：可禁止某些资源隔离功能。支持cpu、memory、netio、diskio
- cpu_config：专门为cpu隔离配置，因cpu有多种策略
  - manage_policy：支持bt（tencent os自带）、cpuset、quota
  - auto_detect：配置为true，则优先选择bt，若不支持，则选择quota方式
  - cpuset_config：若cpu策略为cpuset模式
    - enable_online_isolate：把在线和离线的cpu物理隔离。需要在线作业为非guaranteed类型，且kubelet关闭static策略
    - reserved_cpus：离线可用cpu要去除这些cpu，这些cpu可能是预留给系统组件
  - cpu_quota_config：若cpu策略为quota模式
    - offline_share：离线cpu配置更低的cpu权重

### 8、online
在线作业非运行在K8s平台相关配置

（1）pid_to_cgroup：在线进程移动到统一cgroup目录

- pids_check_interval：每隔多长时间扫描节点上所有的进程，并选择出在线进程pid。在线进程因升级或故障可能重启，从而脱离cgroup目录。
若配置为"0s"，则只在caelus启动过程中扫描一次，之后不会扫描。
- cgroup_check_interval：因扫描节点上所有进程有一定性能开销。可以检测cgroup目录下的进程数目是否发生变化，若发生变化说明在线进程有变化，才触发进程扫描。
该功能在使用场景上只适用于在线进程的数目不发生变化的作业。
- batch_num：匹配在线进程pid过程中，每次匹配的进程数。提升该值，可减少一定的性能开销。

（2）jobs：
在线作业的属性信息

- name：自定义
- command：在线进程命令行的正则表达式 
- metrics：用户自定义指标，如该指标可以是在线进程的时延指标。该配置作用于指标收集模块metrics
  - name：指标名称
    - source：指标来源。支持二进制和url两种方式
      - metrics_command：二进制命令及参数。其返回格式为：
{"msg": "success", "code": 0, "data": [{"{metrics_name}": xxx, "job_name": "xxx", "metric_name": "xxx"}]}, 其中metrics_name为自定义指标名称，可以返回多个指标。
      - cmd_need_chroot：若是二进制模式。caelus在容器里执行该二进制是否需要chroot
      - metrics_url：get请求，其返回格式为：
{"msg": "success", "code": 0, "data": [{"{metrics_name}": xxx, "job_name": "xxx", "metric_name": "xxx"}]}, 其中metrics_name为自定义指标名称，可以返回多个指标。

### 9、alarm
告警配置

（1）ignore_alarm_when_silence：在silence阶段，不发送告警

（2）cluster：集群名称，用于告警时区分不同的集群

（3）message_batch：批量发送，避免频繁发送

（4）message_delay：延迟发送，避免频繁发送

（5）channel_name：支持local和remote
  - local：调用本地二进制发送告警，二进制的输入参数格式为：{"ip":"xx", "cluster":"xx", "alarmMsg":"xx"}
  - remote：调用远程api接口发送
    - remoteWebhook：自定义远程api post接口，其携带的body格式为：{"ip":"xx", "cluster":"xx", "alarmMsg":"xx"}
    - weWorkWebhook：通过企业微信机器人发送

### 10、disk_quota
通过diskquota方式限制pod的磁盘空间使用

（1）container_runtime：目前只支持"docker"， 通过跟dockerd通信获取容器的挂载目录

（2）volume_sizes：支持rootFS(容器根目录)，emptyDir和hostPath
  - quota：磁盘空间大小，单位为bytes
  - inodes：inodes大小