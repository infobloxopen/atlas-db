package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseServer is a specification for a DatabaseServer resource
type DatabaseServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseServerSpec   `json:"spec"`
	Status DatabaseServerStatus `json:"status"`
}

// DatabaseServerSpec is the spec for a DatabaseServer resource
type DatabaseServerSpec struct {
	DatabaseServerPlugin
	Volumes          []corev1.Volume
	RootUserName     string
	RootUserNameFrom *corev1.EnvVarSource
	RootPassword     string
	RootPasswordFrom *corev1.EnvVarSource
	ServicePort      int32
	Port             int32
}

// DatabaseServerPlugin represents the type and method used
// to provision a database server. Only one of its members
// may be set.
type DatabaseServerPlugin struct {
	RDS      *RDSPlugin
	MySQL    *MySQLPlugin
	Postgres *PostgresPlugin
}

// RDSPlugin contains the details needed to provision an RDS instance.
type RDSPlugin struct {
	AllocatedStorage     int32
	Iops                 int32
	DBInstanceClass      string
	DBInstanceIdentifier string
	DBSubnetGroupName    string
	Engine               string
}

// MySQLPlugin contains the details needed to provision a MySQL instance.
type MySQLPlugin struct {
	Image string
	Version string
}

// PostgresPlugin contains the details needed to provision a Postgres instance.
type PostgresPlugin MySQLPlugin

// DatabaseServerStatus is the status for a DatabaseServer resource
type DatabaseServerStatus struct {
	URL string `json:"url"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseServerList is a list of DatabaseServer resources
type DatabaseServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DatabaseServer `json:"items"`
}
