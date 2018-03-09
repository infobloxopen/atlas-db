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

// DatabaseSpec is the spec for a Database resource
type DatabaseSpec struct {
	Users      []DatabaseUser       `json:"users"`
	Dsn        string               `json:"dsn"`
	DsnFrom    *corev1.EnvVarSource `json:"superUserFrom"`
	Server     string               `json:"server"`
	ServerType string               `json:"serverType"`
}

// DatabaseUser represents a user to provision
type DatabaseUser struct {
	Name         string               `json:"name"`
	Password     string               `json:"password"`
	PasswordFrom *corev1.EnvVarSource `json:"passwordFrom"`
	Role         string               `json:"role"`
}

// DatabaseStatus is the status for a Database resource
type DatabaseStatus struct {
	URL string `json:"url"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseList is a list of Database resources
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Database `json:"items"`
}
