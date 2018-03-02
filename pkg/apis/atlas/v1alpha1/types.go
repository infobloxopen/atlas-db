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
	DatabaseServerInstance
	Volumes          []corev1.Volume
	RootUserName     string
	RootUserNameFrom *corev1.EnvVarSource
	RootPassword     string
	RootPasswordFrom *corev1.EnvVarSource
	ServicePort      int32
	Port             int32
}

// DatabaseServerInstance represents the type and method used
// to provision a database server. Only one of its members
// may be set.
type DatabaseServerInstance struct {
	RDS      *RDSInstance
	MySQL    *MySQLInstance
	Postgres *PostgresInstance
}

type LocalDatabaseServer interface {
	PodName() string
	Image() string
	EnvVar() []corev1.EnvVar
	ContainerPort() int32
}

// RDSInstance contains the details needed to provision an RDS instance.
type RDSInstance struct {
	AllocatedStorage     int32
	Iops                 int32
	DBInstanceClass      string
	DBInstanceIdentifier string
	DBSubnetGroupName    string
	Engine               string
}

// MySQLInstance contains the details needed to provision a MySQL instance.
type MySQLInstance struct {
	Image string
}

// PostgresInstance contains the details needed to provision a Postgres instance.
type PostgresInstance MySQLInstance

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
