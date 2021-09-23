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

package predict

import (
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/logic"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/model"
)

// Copied from logic.PodResourceRecommender, delete this file once CPUPercentile and MemoryPeaksPercentile
// are configurable

// PodResourceRecommender computes resource recommendation for a Vpa object.
type PodResourceRecommender interface {
	GetRecommendedPodResources(
		containerNameToAggregateStateMap model.ContainerNameToAggregateStateMap) RecommendedPodResources
}

// RecommendedPodResources is a Map from container name to recommended resources.
type RecommendedPodResources map[string]RecommendedContainerResources

// RecommendedContainerResources is the recommendation of resources for a
// container.
type RecommendedContainerResources struct {
	// Recommended optimal amount of resources.
	Target model.Resources
}

type podResourceRecommender struct {
	types.LocalPredictConfig
	targetEstimator logic.ResourceEstimator
}

// GetRecommendedPodResources computes resource recommendation for a Vpa object.
func (r *podResourceRecommender) GetRecommendedPodResources(
	containerNameToAggregateStateMap model.ContainerNameToAggregateStateMap) RecommendedPodResources {
	var recommendation = make(RecommendedPodResources)
	if len(containerNameToAggregateStateMap) == 0 {
		return recommendation
	}

	fraction := 1.0 / float64(len(containerNameToAggregateStateMap))
	minResources := model.Resources{
		model.ResourceCPU: model.ScaleResource(model.CPUAmountFromCores(r.PodMinCPUMillicores*0.001), fraction),
		model.ResourceMemory: model.ScaleResource(
			model.MemoryAmountFromBytes(r.PodMinMemoryMb*1024*1024), fraction),
	}

	recommender := &podResourceRecommender{
		LocalPredictConfig: r.LocalPredictConfig,
		targetEstimator:    logic.WithMinResources(minResources, r.targetEstimator),
	}

	for containerName, aggregatedContainerState := range containerNameToAggregateStateMap {
		recommendation[containerName] = recommender.estimateContainerResources(aggregatedContainerState)
	}
	return recommendation
}

// Takes AggregateContainerState and returns a container recommendation.
func (r *podResourceRecommender) estimateContainerResources(
	s *model.AggregateContainerState) RecommendedContainerResources {
	return RecommendedContainerResources{
		Target: FilterControlledResources(r.targetEstimator.GetResourceEstimation(s), s.GetControlledResources()),
	}
}

// FilterControlledResources returns estimations from 'estimation' only for resources present in 'controlledResources'.
func FilterControlledResources(estimation model.Resources, controlledResources []model.ResourceName) model.Resources {
	result := make(model.Resources)
	for _, resource := range controlledResources {
		if value, ok := estimation[resource]; ok {
			result[resource] = value
		}
	}
	return result
}

// CreatePodResourceRecommender returns the primary recommender.
func CreatePodResourceRecommender(config types.LocalPredictConfig) PodResourceRecommender {
	targetEstimator := logic.NewPercentileEstimator(config.CPUPercentile, config.MemoryPeaksPercentile)
	targetEstimator = logic.WithMargin(config.SafetyMarginFraction, targetEstimator)

	return &podResourceRecommender{
		LocalPredictConfig: config,
		targetEstimator:    targetEstimator}
}
