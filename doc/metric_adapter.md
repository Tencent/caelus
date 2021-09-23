# metric adapter

Metric adapter is used to collect metrics. Now there are two adapters:

## server adapter

Server adapter provide two apis. Application can report custom metrics by calling the api.
`/appmetrics` report the detail data points, with body:
```json
{
  "name": "{app_name}",
  "namespace": "{namespace}",
  "metric_name": "{metric_name, e.g. latency}",
  "slo": 100.0,
  "sli": 99.9,
  "duration": "5s",
  "data": [11,12,13,14]
}
```

`slo` means the SLO of current application metric, `sli` and `duration` are used to calculate current aggregate metric value.
The calculation method of current aggregate metric value is `sli` percentile of data in the past `duration`.
`data` is the detail data points.
Application can report metrics every second for accuracy and in-time.

`/appslo` is another api that collect aggregated metric data, with body:
```json
{
  "name": "{app_name}",
  "namespace": "{namespace}",
  "metric_name": "{metric_name, e.g. latency}",
  "slo": 100.0,
  "value": 90.0
}
```

`value` is the aggregated metric value. So we can know if current metric is abnormal(`value > slo`).
