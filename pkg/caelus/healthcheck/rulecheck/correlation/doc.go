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

/*
发现异常后的关联分析
同一个（类）指标可能同时会有多个container/pod出现异常，此时优先在这些pod之间进行筛选处理
当然也可能是其他pod影响的，此时主要看最近有无新启动的pod在该指标上有load

发现某个指标有(多个)pod发生异常，首先筛选出其他有load的pod，筛选过滤条件为优先级比较低或者负载比较高的（过滤掉kube-system）
	如果最近(5m)有新起的pod，则优先处理这类pod中load最大的一个，处理结束
	如果没有这类候选项，则选择候选人中优先级较低且load较大的进行处理

对于io异常，各个pod的负载可以通过读写次数来反应
对于内存/cpu，由于用户有各自的申请值，所以无法直接用负载值，优先选择使用值超出起request比例来进行排序
过滤条件，对使用值绝对值有一定要求，比如内存使用大于512M，cpu大于2c

TODO:
1. 处理成功后，对该指标进入一个冷却期（一段时间内不对该指标异常进行分析处理）
2. 记录上报当前结果
*/

package correlation
