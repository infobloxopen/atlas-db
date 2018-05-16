package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
)

const (
	defaultImage   = "postgres"
	defaultVersion = "10.4"
	containerName  = "postgresql"
	portName       = "postgresql"
	defaultPort    = 5432
	dataPath       = "/var/lib/postgresql/data/pgdata"
	superUserPwEnv = "POSTGRES_PASSWORD"
)

type PostgresPlugin atlas.PostgresPlugin

func Convert(a *atlas.PostgresPlugin) *PostgresPlugin {
	p := PostgresPlugin(*a)
	return &p
}

func (p *PostgresPlugin) Name() string {
	return "Postgres"
}

func (p *PostgresPlugin) DatabasePlugin() plugin.DatabasePlugin {
	return p
}

func (p *PostgresPlugin) Dsn(pw string, s *atlas.DatabaseServer) string {
	return fmt.Sprintf("%s:%s@%s.%s:%d/postgres?sslmode=disable",
		s.Spec.SuperUser,
		pw,
		s.Name,
		s.Namespace,
		s.Spec.Port)
}

// CreatePod creates a new Pod for a DatabaseServer resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DatabaseServer resource that 'owns' it.
func (p *PostgresPlugin) CreatePod(key string, s *atlas.DatabaseServer) *corev1.Pod {
	labels := map[string]string{
		"controller": s.Name,
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
							ContainerPort: plugin.PodContainerPort(s.Spec.Port, defaultPort),
						},
					},
				},
			},
		},
	}
	c := &pod.Spec.Containers[0]

	if p.DataVolume != "" {
		plugin.AddMount(c, p.DataVolume, false, dataPath)
	}

	return pod
}

// DiffPod returns a string describing differences between the spec and the pod,
// or the empty string if there are no differences
func (p *PostgresPlugin) DiffPod(key string, s *atlas.DatabaseServer, pod *corev1.Pod) string {
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

func (p *PostgresPlugin) SyncDatabase(db *atlas.Database, dsn string) error {
	// connect
	sqldb, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	// check if it exists; if not create it
        rows, err := sqldb.Query(`SELECT 1 FROM pg_database WHERE datname=$1`, db.Name)
	if err != nil {
		return err
	}

	if rows.Next() {
		return nil
	}

	ddl := fmt.Sprintf(`create database "%s"`, db.Name)
	_, err = sqldb.Exec(ddl)
	if err != nil {
		return err
	}
	// check if users exist; verify their passwords and roles
	// check if other users exist; delete if necessary

	return nil
}

func (p *PostgresPlugin) DeleteDatabase(db *atlas.Database) error {
	// connect
	// delete users
	// delete database
	return nil
}
