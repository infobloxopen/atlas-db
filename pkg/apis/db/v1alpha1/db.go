package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Database is a specification for a Database resource
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec"`
	Status DatabaseStatus `json:"status"`
}

// DatabaseSpec is the spec for a DatabaseServer resource
type DatabaseSpec struct {
	UserList []DatabaseUser          `json:"userList"`
	DsnFrom  *corev1.SecretReference `json:"dsnFrom"`
}

// DatabaseUser represents
type DatabaseUser struct {
	Name         string                  `json:"name"`
	Role         string                  `json:"role,omitempty"`
	Password     string                  `json:"password,omitempty"`
	PasswordFrom *corev1.SecretReference `json:"passwordFrom,omitempty"`
}

// DatabaseStatus is the status for a DatabaseServer resource
type DatabaseStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseList is a list of DatabaseServer resources
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Database `json:"items"`
}
