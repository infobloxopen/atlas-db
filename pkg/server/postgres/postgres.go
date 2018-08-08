package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/golang/glog"
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
	admin_role     = "createrole createdb"
	read_role      = "nocreaterole nocreatedb"
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

func (p *PostgresPlugin) Dsn(userName string, password string, db *atlas.Database, s *atlas.DatabaseServer) string {
	if userName == "" {
		userName = s.Spec.SuperUser
	}
	if password == "" {
		password = s.Spec.SuperUserPassword
	}

	dbHost := fmt.Sprintf("%s.%s", s.Name, s.Namespace)
	if s.Spec.Host != "" {
		dbHost = s.Spec.Host
	}

	// For superuser DSN creation database name will not be passed &
	// for admin DSN creation database name will be passed
	database := "postgres"
	if db != nil {
		database = db.Name
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		userName,
		password,
		dbHost,
		s.Spec.ServicePort,
		database)
}

// CreatePod creates a new Pod for a DatabaseServer resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DatabaseServer resource that 'owns' it.
func (p *PostgresPlugin) CreatePod(key string, s *atlas.DatabaseServer) *corev1.Pod {
	labels := s.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["controller"] = s.Name
	labels["databaseserver"] = s.Name

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
	defer sqldb.Close()
	// check if it exists; if not create it
	rows, err := sqldb.Query(`SELECT 1 FROM pg_database WHERE datname=$1`, db.Name)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		glog.V(4).Infof("Database %s exists", db.Name)
		err = checkUsers(db, sqldb)
		if err != nil {
			return err
		}
		return nil
	}
	glog.V(4).Info("creating database ", db.Name)
	ddl := fmt.Sprintf(`create database "%s"`, db.Name)
	_, err = sqldb.Exec(ddl)
	if err != nil {
		return err
	}
	for _, user := range db.Spec.Users {
		err = createuser(user.Name, user.Role, user.Password, sqldb)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresPlugin) DeleteDatabase(db *atlas.Database) error {
	// connect
	// delete users
	// delete database
	return nil
}

func checkUsers(db *atlas.Database, sqldb *sql.DB) error {
	//// check if users exist; verify their passwords and roles
	dbUserMap := make(map[string]string)
	var u string
	//getting all users from the database
	rows, err := sqldb.Query(`SELECT u.usename FROM pg_catalog.pg_user u`)
	if err != nil {
		return err
	}
	for rows.Next() {
		if err := rows.Scan(&u); err != nil {
			return err
		}
		dbUserMap[u] = "delete-me"
	}
	glog.V(4).Info("Users exist in database are ", dbUserMap)
	users := db.Spec.Users
	glog.V(4).Info("Users from yaml are ", users)

	for _, user := range users {
		if _, ok := dbUserMap[user.Name]; !ok {
			err = createuser(user.Name, user.Role, user.Password, sqldb)
			if err != nil {
				return err
			}
		} else {
			dbUserMap[user.Name] = "dont-delete"
			if user.Role == "admin" {
				//roles now assigned to admin is createdb, createrole
				_, err = sqldb.Exec(fmt.Sprintf(`alter user "%s" with %s`, user.Name, admin_role))
				if err != nil {
					return err
				}
			} else if user.Role == "read" {
				_, err = sqldb.Exec(fmt.Sprintf(`alter user "%s" with %s`, user.Name, read_role))
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("role for the user %s is not valid", user.Name)
			}
			//There is no straight forward ways to compare password so just updating it
			_, err = sqldb.Exec(fmt.Sprintf(`alter user "%s" with password '%s'`, user.Name, user.Password))
			if err != nil {
				return err
			}
		}
	}
	//TODO: below will delete all the users in the database. need to delete users for particular app.
	// delete users
	//for dbUserName, dbUserVal := range dbUserMap {
	//	//if delete-me delete database user
	//	if dbUserVal == "delete-me" && dbUserName != "postgres" {
	//		glog.Info("deleted user ", dbUserName)
	//		err = deleteUser(dbUserName, sqldb)
	//		if err != nil {
	//			return err
	//		}
	//	}
	//}
	return nil
}

func createuser(username, userrole, password string, sqldb *sql.DB) error {
	if userrole == "admin" {
		_, err := sqldb.Exec(fmt.Sprintf(`create user "%s" with password '%s' %s`, username, password, admin_role))
		if err != nil {
			return err
		}
	} else if userrole == "read" {
		_, err := sqldb.Exec(fmt.Sprintf(`create user "%s" with password '%s' %s`, username, password, read_role))
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("role for the user %s is not valid", username)
	}
	return nil
}

func deleteUser(userName string, sqldb *sql.DB) error {
	ddl := fmt.Sprintf(`drop owned by "%s"; drop user "%s"`, userName, userName)
	_, err := sqldb.Exec(ddl)
	if err != nil {
		return err
	}
	return nil
}
