package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AppRole is a resource that represents a role in a specific application
type AppRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppRoleSpec   `json:"spec"`
	Status AppRoleStatus `json:"status"`
}

type Criterion struct {
	Key string
	Op string
	Value string
	ValueRef string
}

// AppRoleSpec is the spec for an AppRole resource
type AppRoleSpec struct {
	Application string
	AccountID string
	Criteria []Criterion
}

// AppRoleStatus contains the status for the AppRole resource
type AppRoleStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseServerList is a list of DatabaseServer resources
type AppRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []AppRole `json:"items"`
}
