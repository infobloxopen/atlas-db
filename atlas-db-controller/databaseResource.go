package main

import (
	"fmt"

	"strconv"
	"strings"

	"github.com/golang/glog"
	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	"github.com/infobloxopen/atlas-db/pkg/server"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

func (c *Controller) enqueueDatabase(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		return
	}
	glog.Infof("enqueue database object: %s", object.GetName())
	c.enqueue(obj, c.dbQueue)
}

func (c *Controller) syncDatabase(key string) error {
	glog.Infof("Processing database : %v", key)
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Database resource with this namespace/name
	db, err := c.dbsLister.Databases(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("database '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	if db.Status.State == "" {
		c.updateDatabaseStatus(key, db, StatePending, "Yet to initialize")
	}

	var p plugin.DatabasePlugin
	var s *atlas.DatabaseServer

	if db.Spec.Server != "" {
		// TODO this implies the same namespace for database and server. Do we want this?
		s, err = c.serversLister.DatabaseServers(namespace).Get(db.Spec.Server)
		if err != nil {
			if errors.IsNotFound(err) {
				msg := fmt.Sprintf("waiting for database server '%s/%s'", namespace, db.Spec.Server)
				glog.Info(msg)
				c.updateDatabaseStatus(key, db, StatePending, msg)
			} else {
				runtime.HandleError(fmt.Errorf("error retrieving database server '%s' for database '%s': %s", db.Spec.Server, key, err))
			}
			// requeue
			return err
		}
	}

	serverType := db.Spec.ServerType
	if serverType == "" && s == nil {
		msg := fmt.Sprintf("database '%s' has no serverType or server set", key)
		glog.Error(msg)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return nil
	}

	if serverType != "" {
		p = server.NewDBPlugin(serverType)
	} else {
		p = server.ActivePlugin(s).DatabasePlugin()
	}

	if p == nil {
		msg := fmt.Sprintf("database '%s' does not have a valid database plugin", key)
		glog.Error(msg)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return nil
	}

	// If dsn/dsnFrom is passed in the database spec consider as override and don't go through database spec
	dsn := db.Spec.Dsn
	if dsn == "" {
		if db.Spec.DsnFrom != nil {
			secretName := db.Spec.DsnFrom.SecretKeyRef.Name
			dsn, err = c.getSecretFromValueSource(db.Namespace, db.Spec.DsnFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting to get DSN for database `%s` from secret `%s`", key, secretName)
					glog.Info(msg)
					c.updateDatabaseStatus(key, db, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid DSN for database `%s` from secret `%s`", key, secretName)
				glog.Error(msg)
				c.updateDatabaseStatus(key, db, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		} else { // Get the dsn with superuser info from database server created secret
			dsn, err = c.getSecretByName(db.Namespace, "dsn", s.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting to get DSN for database `%s` from secret `%s`", key, s.Name)
					glog.Info(msg)
					c.updateDatabaseStatus(key, db, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid DSN for database `%s` from secret `%s`", key, s.Name)
				glog.Error(msg)
				c.updateDatabaseStatus(key, db, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		}
	}

	// Update dsn related to a database which databaseschema will use.
	for index, user := range db.Spec.Users {
		if user.PasswordFrom != nil {
			passwd, err := c.getSecretFromValueSource(db.Namespace, user.PasswordFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting for secret or configmap for %s", user.Name)
					c.updateDatabaseStatus(key, db, StatePending, msg)
					return err
				}
			}
			db.Spec.Users[index].Password = passwd
		}
	}

	err = p.SyncDatabase(db, dsn)
	if err != nil {
		msg := fmt.Sprintf("error syncing database '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}
	msg := fmt.Sprintf("Database '%s' & users synced successfully", s.Name)
	c.recorder.Event(db, corev1.EventTypeNormal, SuccessSynced, msg)

	err = c.syncDatabaseSecret(key, dsn, db, s, p)
	if err != nil {
		msg := fmt.Sprintf("error syncing database secrets '%s': %s", key, err)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return nil
	}

	c.updateDatabaseStatus(key, db, StateSuccess, fmt.Sprintf(MessageDatabaseSynced, key))
	return nil
}

func (c *Controller) syncDatabaseSecret(key, dsn string, db *atlas.Database, dbServer *atlas.DatabaseServer, dbPlugin plugin.DatabasePlugin) error {
	if db.Spec.Users == nil {
		glog.V(4).Info(" Database users not provided. Skip database secret creation")
		return nil
	}
	secret, err := c.secretsLister.Secrets(db.Namespace).Get(db.Name)
	//TODO: check if the secret matches the spec and change it if not
	//this will require additional support from the database plugin
	if err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to get secret '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	// If the resource doesn't exist, we'll create it.
	// TODO: creating dsn for admin user alone for now. non-admin users also we should create.
	for _, user := range db.Spec.Users {
		passwd := user.Password
		if user.Role == "admin" {
			if errors.IsNotFound(err) {
				glog.V(4).Info("Database secrets not found. Creating...")
				if dbServer != nil {
					dsn = dbPlugin.Dsn(user.Name, passwd, db, dbServer)
				} else {
					customDbServer := &atlas.DatabaseServer{}
					host, port := c.getHostAndPort(dsn)
					customDbServer.Spec.DBHost = host
					customDbServer.Spec.ServicePort = port
					dsn = dbPlugin.Dsn(user.Name, passwd, db, customDbServer)
				}

				secret, err = c.kubeclientset.CoreV1().Secrets(db.Namespace).Create(
					&corev1.Secret{
						ObjectMeta: c.objMeta(db, "Secret"),
						StringData: map[string]string{"dsn": dsn},
					},
				)
				// If an error occurs during Create, we'll requeue the item so we can
				// attempt processing again later.
				if err != nil {
					msg := fmt.Sprintf("failed to create secret '%s': %s", key, err)
					glog.Error(msg)
					c.updateDatabaseStatus(key, db, StateError, msg)
					runtime.HandleError(fmt.Errorf(msg))
					return err
				}
				c.recorder.Event(db, corev1.EventTypeNormal, StateCreated, fmt.Sprintf(MessageSecretCreated, secret.Name))
			}
		}
	}

	// If it is not controlled by this Database resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(secret, db) {
		msg := fmt.Sprintf(MessageSecretExists, secret.Name)
		glog.Info(msg)
		c.recorder.Event(db, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// TODO: compare existing to spec and reconcile
	return nil
}

func (c *Controller) getHostAndPort(dsn string) (host string, port int32) {
	splitDSN := strings.Split(strings.Split(dsn, "@")[1], "/")[0]
	host = strings.Split(splitDSN, ":")[0]
	portInt, _ := strconv.Atoi(strings.Split(splitDSN, ":")[1])
	port = int32(portInt)
	return
}

func (c *Controller) updateDatabaseStatus(key string, db *atlas.Database, state, msg string) (*atlas.Database, error) {
	copy := db.DeepCopy()
	copy.Status.State = state
	copy.Status.Message = msg
	// Until #38113 is merged, we must use Update instead of UpdateStatus to
	// update the Status block of the resource. UpdateStatus will not
	// allow changes to the Spec of the resource, which is ideal for ensuring
	// nothing other than resource status has been updated.
	_, err := c.atlasclientset.AtlasdbV1alpha1().Databases(db.Namespace).Update(copy)
	if err != nil {
		runtime.HandleError(fmt.Errorf("error updating status to '%s' for database '%s': %s", state, key, err))
		return db, err
	}
	// we have to pull it back out or our next update will fail. hopefully this is fixed with updateStatus
	return c.dbsLister.Databases(db.Namespace).Get(db.Name)
}
