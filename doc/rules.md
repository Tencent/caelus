# Detection configuration

  Caelus dynamically checks abnormalities of various metrics based on [rules.json](../hack/config/rules.json), such as CPU 
  usage or online latency, to make sure online jobs run normally. Batch jobs will be throttled or even killed if 
  interference detected.
  This document describes how to configure [rules.json](../hack/config/rules.json)

## 1.Rules
  Rules are used to assign the algorithm to check if the timed metrics have significant fluctuations, such as EWMA, 
  and then take actions to throttle batch jobs, such as disable scheduling, or decrease idle resources.
  
### Detects
  Detects are used to check if the timed metrics have significant fluctuations. For now, the supported detecting 
  algorithms are as following:
  
#### EWMA(exponentially weighted moving average)
  EWMA algorithm is used to check if the timed metrics are fluctuating rapidly, it is useful for checking smooth metrics.
  Users should assign the metrics number(nr).
```json
{
  "name": "ewma",
  "args": {
    "metric": "read_latency",
    "nr":100
  }
}
```
#### Expression
  Expression algorithm is used to check if the timed metrics are higher or lower than a fixed threshold for a while.
  Users should assign the time by the way of how many times(warning_count), or how long the period(warning_duration).
```json
{
  "name": "expression",
  "args": {
    "expression": "cpu_avg > 0.9",
    "warning_count": 10,
    "warning_duration": "10s"
    }
}
```
  
### Actions
  Actions are used to throttle batch jobs when resource pressure or latency detected. For now, the supported actions 
  are as following:
  
#### Adjust resource
  Adjust resource will decrease idle resources in "step". Users could assign multiple resources,
  such as CPU or memory, the "op" decides if to handle all resources with the value "all", or just one kind of resources
  with the value "loop", the default value is "all".
```json
{
  "name": "adjust",
  "args": {
    "op": "loop",
    "resources": [
      {
        "resource": "cpu",
        "step": "1000m"
      }
    ]
  }
}
```
#### Evict batch job
  TODO
  
#### Warning
  This action just sends alarm warning.
```json
{
  "name": "log",
  "args": {}
}
```

#### Disable schedule  
  The Schedule action will disable scheduling batch jobs for the node.
```json
{
  "name": "schedule",
  "args": {}
}
```

## 2.Rules check
  The Rule decides which metrics to detect, with a different combination of detection algorithms and actions. Users could
  assign multiple rules, and for now, they are separated into nodes rules, container rules, and app rules.
  
  Users should assign different intervals, including "check_intervals", which indicates the interval to call detection
  algorithm; "handle_interval", which indicates the interval to trigger handle actions; "recover_interval", which indicates 
  the interval to recover when metrics is normal again.

### Node rules
  Node rules describe how to detect metrics of node level, the supported metrics could be found from
   [node metrics](../pkg/caelus/statestore/common/node/type.go), like "cpu_avg" as following:
```go
type NodeCpu struct {
	...
	// core used per second
	CpuTotal   float64   `structs:"cpu_total"`
	CpuPerCore []float64 `structs:"cpu_per_core"`
	CpuAvg     float64   `structs:"cpu_avg"`
	...
}
```   
  Node rules example:
```json
      {
        "name": "cpu",
        "metrics": [
            "cpu_avg"
        ],
        "check_interval": "10s",
        "handle_interval": "10s",
        "recover_interval": "15s",
        "rules": [
          {
            "detects": [
              {
                "name": "expression",
                "args": {
                  "expression": "cpu_avg > 0.9",
                  "warning_count": 10,
                  "warning_duration": "10s"
                }
              }
            ],
            "actions": [
              {
                "name": "adjust",
                "args": {
                  "resources": [
                    {
                      "step": "1000m"
                    }
                  ]
                }
              },
              {
                "name": "schedule",
                "args": {}
              }
            ]
          }
        ]
      }
```

### Container rules
  Container rules describe how to detect metrics of container level, the supported metrics could be found from
   [container metrics](../pkg/caelus/statestore/cgroup/types.go), like "nr_cpu_throttled" as following:
```go
// CgroupStats contains cgroup stats
type CgroupStats struct {
    ...
	NrCpuThrottled           float64    `structs:"nr_cpu_throttled"`
    ...
}
```

  Container rules example:
```json
{
 "metrics": [
     "nr_cpu_throttled"
 ],
 "check_interval": "5s",
 "handle_interval": "10s",
 "recover_interval": "15s",
 "rules": [
   {
     "detects": [
       {
         "name": "expression",
         "args": {
           "expression": "nr_cpu_throttled > 0"
         }
       }
     ]
   }
 ]
}
```
### App rules
  App rules describe how to detect metrics of app level, the metrics are provided by users themselves, in the way of 
  executable command or http server, the example as flowing: 
```json
{
 "enable": false,
 "pid_to_cgroup": {
   "pids_check_interval": "0s",
   "cgroup_check_interval": "1m",
   "batch_num": 50
 },
 "jobs": [
   {
     "name": "xxx",
     "command": "xxx",
     "metrics": [
       {
         "name": "latency",
         "source": {
           "check_interval": "1m",
           "metrics_command": ["/xxx"],
           "cmd_need_chroot": false,
           "metrics_url": ""
         }
       }
     ]
   }
 ]
}
```
  The supported metrics are result of "metrics_command" or "metrics_url", codes can be found from 
  [app metrics](../pkg/caelus/statestore/common/customize/types.go). The format of rule name is `{jobName}:{metricName}`.
  `jobName` and `metricName` are come from the online configuration section. There is a special case case where the name
  is `slo`, with two corresponding metrics `slo_min`, `slo_max`.
  App rules example:
```json
[
  {
    "name": "osd:latency",
    "metrics": [
      "slo_min",
      "slo_max"
    ],
    "check_interval": "5s",
    "handle_interval": "10s",
    "recover_interval": "15s",
    "rules": [
      {
        "detects": [
          {
            "name": "expression",
            "args": {
              "expression": "slo_min < 0.1"
            }
          }
        ],
        "actions": [
          {
            "name": "schedule",
            "args": {}
          }
        ]
      }
    ],
    "recover_rules": [
      {
        "detects": [
          {
            "name": "expression",
            "args": {
              "expression": "slo_max < 0.2"
            }
          }
        ]
      }
    ]
  },
  {
   "name": "osd:latency",
   "metrics": [
     "value",
     "slo",
     "count"
   ],
   "check_interval": "5s",
   "handle_interval": "10s",
   "recover_interval": "15s",
   "rules": [
     {
       "detects": [
         {
           "name": "expression",
           "args": {
             "expression": "value > slo && count > 100",
             "warning_count": 3
           }
         }
       ],
       "actions": [
         {
           "name": "schedule",
           "args": {}
         }
       ]
     }
   ],
   "recover_rules": [
     {
       "detects": [
         {
           "name": "expression",
           "args": {
             "expression": "value < 0.9*slo",
             "warning_count": 3
           }
         }
       ]
     }
   ]
  }
]
```  

## 3.PSI
  Caelus supports using PSI(Pressure Stall Information) mechanism to detect resource pressure, which is more quickly.
  
### Memory cgroup event
  Memory cgroup event could be found from [here](https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt). Caelus 
  are using "memory.pressure_level" and "memory.usage_in_bytes" to listen if the memory resource is under heavy load.
  The "pressure_level" indicates the memory pressure level, which could be only one of "low|medium|critical". The "margin_mb"
  indicates how much memory is left to trigger action.
  
```json
{
 "pressures": [
    {
      "cgroups": [
         "/kubepods/offline"
      ],
      "pressure_level": "low",
      "duration": "30ms",
      "count": 2
    }
  ],
  "usages": [
    {
      "cgroups": [
        "/kubepods/offline/test"
      ],
      "margin_mb": 2048,
      "duration": "1m"
    }
  ]
}
```  
