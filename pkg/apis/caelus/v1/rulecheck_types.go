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

package v1

import (
	"github.com/tencent/caelus/pkg/util/times"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RuleCheckType string

const (
	AppType       RuleCheckType = "app"
	NodeType      RuleCheckType = "node"
	ContainerType RuleCheckType = "container"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RuleCheck is a specification for a RuleCheck resource
type RuleCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuleCheckSpec   `json:"spec"`
	Status RuleCheckStatus `json:"status,omitempty"`
}

// RuleCheckSpec is the spec for a RuleCheck resource
type RuleCheckSpec struct {
	Name         string            `json:"name"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	Priority     int32             `json:"priority,omitempty"`
	Type         RuleCheckType     `json:"type"`
	Metrics      []string          `json:"metrics"`

	// CheckInterval describes the interval to trigger detection
	CheckInterval *times.Duration `json:"checkInterval"`
	// HandleInterval describes the interval to handle conflicts after detecting abnormal result
	HandleInterval *times.Duration `json:"handleInterval"`
	// RecoverInterval describes the interval to recover conflicts after detecting normal result
	RecoverInterval *times.Duration `json:"recoverInterval"`

	Rules        []*DetectActionRules `json:"rules"`
	RecoverRules []*DetectActionRules `json:"recoverRules"`
}

// DetectActionRules define detectors and actions
type DetectActionRules struct {
	Detects []*DetectAction `json:"detects"`
	Actions []*DetectAction `json:"actions"`
}

// DetectAction define detector config
type DetectAction struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

// RuleCheckStatus is the status for a RuleCheck resource
type RuleCheckStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RuleCheckList is a list of RuleCheck resources
type RuleCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RuleCheck `json:"items"`
}
