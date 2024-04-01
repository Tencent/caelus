package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tencent/caelus/pkg/util/times"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CgroupNotify is a specification for a CgroupNotify resource
type CgroupNotify struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CgroupNotifySpec   `json:"spec"`
	Status CgroupNotifyStatus `json:"status,omitempty"`
}

// CgroupNotifySpec is the spec for a CgroupNotify resource
type CgroupNotifySpec struct {
	MemoryCgroup MemoryNotifyConfig `json:"memory_cgroup"`
	NodeSelector map[string]string  `json:"nodeSelector,omitempty"`
	Priority     int32              `json:"priority,omitempty"`
}

// MemoryNotifyConfig describe memory cgroup notify
type MemoryNotifyConfig struct {
	Pressures []MemoryPressureNotifyConfig `json:"pressures"`
	Usages    []MemoryUsageNotifyConfig    `json:"usages"`
}

// MemoryPressureNotifyConfig describe memory.pressure_level notify data
type MemoryPressureNotifyConfig struct {
	Cgroups       []string `json:"cgroups"`
	PressureLevel string   `json:"pressure_level"`
	// assign time duration the pressure has kept
	Duration times.Duration `json:"duration"`
	// assign event number in the duration time
	Count int `json:"count"`
}

// MemoryUsageNotifyConfig describe memory.usage_in_bytes notify data
type MemoryUsageNotifyConfig struct {
	Cgroups []string `json:"cgroups"`
	// the distance between limit and threshold
	MarginMb int `json:"margin_mb"`
	// when to handle event after receiving event
	Duration times.Duration `json:"duration"`
}

// CgroupNotifyStatus is the status for a CgroupNotify resource
type CgroupNotifyStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CgroupNotifyList is a list of CgroupNotify resources
type CgroupNotifyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CgroupNotify `json:"items"`
}
