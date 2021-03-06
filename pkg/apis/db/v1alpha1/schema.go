package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseSchema is a specification for a DatabaseSchema resource
type DatabaseSchema struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSchemaSpec   `json:"spec"`
	Status DatabaseSchemaStatus `json:"status"`
}

// DatabaseSchemaSpec is the spec for a DatabaseSchema resource
type DatabaseSchemaSpec struct {
	Database   string       `json:"database"`
	Dsn        string       `json:"dsn"`
	DsnFrom    *ValueSource `json:"dsnFrom"`
	Source     string       `json:"source"`
	SourceFrom *ValueSource `json:"sourceFrom"`
	Version    int          `json:"version"` // version of database schema
}

// DatabaseSchemaStatus is the status for a DatabaseSchema resource
type DatabaseSchemaStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseSchemaList is a list of DatabaseSchema resources
type DatabaseSchemaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DatabaseSchema `json:"items"`
}
