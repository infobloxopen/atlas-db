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
	Volumes               []corev1.Volume `json:"volumes"`
	SuperUser             string          `json:"superUser"`
	SuperUserFrom         *ValueSource    `json:"superUserFrom"`
	SuperUserPassword     string          `json:"superUserPassword"`
	SuperUserPasswordFrom *ValueSource    `json:"superUserPasswordFrom"`
	Host                  string          `json:"host"`
	ServicePort           int32           `json:"servicePort"`
}

// DatabaseServerPlugin represents the type and method used
// to provision a database server. Only one of its members
// may be set.
type DatabaseServerPlugin struct {
	RDS      *RDSPlugin      `json:"rds"`
	MySQL    *MySQLPlugin    `json:"mySQL"`
	Postgres *PostgresPlugin `json:"postgres"`
}

// RDSPlugin contains the details needed to provision an RDS instance.
type RDSPlugin struct {
	AllocatedStorage     int32  `json:"allocatedStorage"`
	Iops                 int32  `json:"iops"`
	DBInstanceClass      string `json:"dbInstanceClass"`
	DBInstanceIdentifier string `json:"dbInstanceIdentifier"`
	DBSubnetGroupName    string `json:"dbSubnetGroupName"`
	Engine               string `json:"engine"`
}

// MySQLPlugin contains the details needed to provision a MySQL instance.
type MySQLPlugin struct {
	Image        string `json:"image"`
	Version      string `json:"version"`
	DataVolume   string `json:"dataVolume"`
	ConfigVolume string `json:"configVolume"`
}

// PostgresPlugin contains the details needed to provision a Postgres instance.
type PostgresPlugin struct {
	Image      string `json:"image"`
	Version    string `json:"version"`
	DataVolume string `json:"dataVolume"`
	InitdbArgs string `json:"initdbArgs"`
}

// DatabaseServerStatus is the status for a DatabaseServer resource
type DatabaseServerStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseServerList is a list of DatabaseServer resources
type DatabaseServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DatabaseServer `json:"items"`
}
