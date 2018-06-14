package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
)

const (
	defaultImage   = "mysql"
	defaultVersion = "5.6"
	containerName  = "mysqld"
	portName       = "mysql"
	defaultPort    = 3306
	configPath     = "/etc/mysql/conf.d"
	dataPath       = "/var/lib/mysql"
	superUserPwEnv = "MYSQL_ROOT_PASSWORD"
)

type MySQLPlugin atlas.MySQLPlugin

func Convert(a *atlas.MySQLPlugin) *MySQLPlugin {
	p := MySQLPlugin(*a)
	return &p
}

func (p *MySQLPlugin) Name() string {
	return "MySQL"
}

func (p *MySQLPlugin) DatabasePlugin() plugin.DatabasePlugin {
	return p
}

func (p *MySQLPlugin) Dsn(userName string, password string, db *atlas.Database, s *atlas.DatabaseServer) string {
	return fmt.Sprintf("%s:%s@tcp(%s.%s:%d)/mysql?charset=utf8&parseTime=True",
		userName,
		password,
		s.Name,
		s.Namespace,
		s.Spec.ServicePort)
}

// CreatePod creates a new Pod for a DatabaseServer resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DatabaseServer resource that 'owns' it.
func (p *MySQLPlugin) CreatePod(key string, s *atlas.DatabaseServer) *corev1.Pod {
	labels := map[string]string{
		"controller":     s.Name,
		"databaseserver": s.Name,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(s, schema.GroupVersionKind{
					Group:   atlas.SchemeGroupVersion.Group,
					Version: atlas.SchemeGroupVersion.Version,
					Kind:    "DatabaseServer",
				}),
			},
		},
		Spec: corev1.PodSpec{
			Volumes: s.Spec.Volumes,
			Containers: []corev1.Container{
				{
					Name:  containerName,
					Image: plugin.PodImage(p.Image, p.Version, defaultImage, defaultVersion),
					Env: []corev1.EnvVar{
						{
							Name:      superUserPwEnv,
							Value:     s.Spec.SuperUserPassword,
							ValueFrom: s.Spec.SuperUserPasswordFrom.ToEnvVarSource(),
						},
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          portName,
							Protocol:      "TCP",
							ContainerPort: plugin.PodContainerPort(s.Spec.ServicePort, defaultPort),
						},
					},
				},
			},
		},
	}
	c := &pod.Spec.Containers[0]
	if p.ConfigVolume != "" {
		plugin.AddMount(c, p.ConfigVolume, true, configPath)
	}

	if p.DataVolume != "" {
		plugin.AddMount(c, p.DataVolume, false, dataPath)
	}

	return pod
}

// DiffPod returns a string describing differences between the spec and the pod,
// or the empty string if there are no differences
func (p *MySQLPlugin) DiffPod(key string, s *atlas.DatabaseServer, pod *corev1.Pod) string {
	diffs := []string{}
	var c *corev1.Container
	if len(pod.Spec.Containers) != 1 {
		diffs = append(diffs, "incorrect number of containers")
	} else {
		c = &pod.Spec.Containers[0]
	}

	expImg := plugin.PodImage(p.Image, p.Version, defaultImage, defaultVersion)
	if c != nil && c.Image != expImg {
		diffs = append(diffs, "incorrect image")
	}

	//TODO: Check volumes, etc.
	return strings.Join(diffs, "; ")
}

func (p *MySQLPlugin) SyncDatabase(db *atlas.Database, dsn string) error {
	// connect
	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	// check if it exists; if not create it
	ddl := fmt.Sprintf("create database if not exists `%s`", db.Name)
	_, err = sqldb.Exec(ddl)
	if err != nil {
		return err
	}
	// check if users exist; verify their passwords and roles
	// check if other users exist; delete if necessary

	return nil
}

func (p *MySQLPlugin) DeleteDatabase(db *atlas.Database) error {
	// connect
	// delete users
	// delete database
	return nil
}
