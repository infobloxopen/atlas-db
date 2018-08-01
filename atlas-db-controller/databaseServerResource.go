package main

import (
	"fmt"

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

const (
	MessageServiceExists = "Service (%q) already exists and is not managed by DatabaseServer"
	MessagePodExists     = "Pod (%q) already exists and is not managed by DatabaseServer"
	MessageServerSynced  = "DatabaseServer (%q) synced successfully"
)

func (c *Controller) enqueueDatabaseServer(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		glog.Info("not enqueue server object")
		return
	}
	glog.Infof("enqueue server object: %s", object.GetName())
	c.enqueue(obj, c.serverQueue)
}

// syncServer compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the DatabaseServer resource
// with the current status of the resource.
func (c *Controller) syncServer(key string) error {
	glog.Infof("Processing database server : %v", key)
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the DatabaseServer resource with this namespace/name
	s, err := c.serversLister.DatabaseServers(namespace).Get(name)
	if err != nil {
		// The DatabaseServer resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("databaseserver '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	p := server.ActivePlugin(s)

	glog.V(4).Infof("DatabaseServer %s has plugin type: %T", key, p)
	err = fmt.Errorf("databaseserver '%s' has an unimplemented plugin type", key)

	if pp, ok := p.(plugin.PodPlugin); ok {
		err = c.syncPodServer(pp, key, s)
		if err != nil {
			msg := fmt.Sprintf("error syncing server pod '%s': %s", key, err)
			glog.Error(msg)
			c.updateDatabaseServerStatus(key, s, StateError, msg)
			runtime.HandleError(fmt.Errorf(msg))
			return err
		}
	}

	/* TODO: Currently not supporting RDS instance provisioning
	if cp, ok := p.(plugin.CloudPlugin); ok {
		err = cp.SyncCloud(key, s) // This function also not implemented
	}
	if err != nil {
		return err
	}
	*/
	err = c.syncService(key, s)
	if err != nil {
		msg := fmt.Sprintf("error syncing service '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseServerStatus(key, s, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	err = c.syncServerSecret(key, s)
	if err != nil {
		msg := fmt.Sprintf("error syncing server secret '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseServerStatus(key, s, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	//TODO: Add a check to see if the db server is up and running and accessible via the Service and Secret
	c.updateDatabaseServerStatus(key, s, StateSuccess, fmt.Sprintf(MessageServerSynced, key))
	return nil
}

func (c *Controller) syncServerSecret(key string, s *atlas.DatabaseServer) error {
	// Creates a secret with the same name as the server.
	secret, err := c.secretsLister.Secrets(s.Namespace).Get(s.Name)
	// If an error occurs during Get, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to get secret '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseServerStatus(key, s, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	//TODO: check if the secret matches the spec and change it if not
	// this will require additional support from the database plugin

	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		glog.V(4).Infof("Database Server secret not found for %s. Creating...", key)
		su := s.Spec.SuperUser
		if su == "" {
			su, err = c.getSecretFromValueSource(s.Namespace, s.Spec.SuperUserFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := "waiting for secret or configmap for superUser"
					glog.Info(msg)
					c.updateDatabaseServerStatus(key, s, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("databaseserver '%s' has no valid superUser or source", key)
				glog.Error(msg)
				c.updateDatabaseServerStatus(key, s, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		}

		supw := s.Spec.SuperUserPassword
		if supw == "" {
			supw, err = c.getSecretFromValueSource(s.Namespace, s.Spec.SuperUserPasswordFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := "waiting for secret or configmap for superUserPassword"
					glog.Info(msg)
					c.updateDatabaseServerStatus(key, s, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("databaseserver '%s' has no valid superUserPassword or source", key)
				glog.Error(msg)
				c.updateDatabaseServerStatus(key, s, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		}

		dsn := server.ActivePlugin(s).DatabasePlugin().Dsn(su, supw, nil, s)
		secret, err = c.kubeclientset.CoreV1().Secrets(s.Namespace).Create(
			&corev1.Secret{
				ObjectMeta: c.objMeta(s, "Secret"),
				StringData: map[string]string{"dsn": dsn},
			},
		)
		// If an error occurs during Create, we'll requeue the item so we can
		// attempt processing again later.
		if err != nil {
			msg := fmt.Sprintf("failed to create secret '%s': %s", key, err)
			glog.Error(msg)
			c.updateDatabaseServerStatus(key, s, StateError, msg)
			runtime.HandleError(fmt.Errorf(msg))
			return err
		}
		c.recorder.Event(s, corev1.EventTypeNormal, StateCreated, fmt.Sprintf(MessageSecretCreated, secret.Name))
	}

	// If it is not controlled by this DatabaseServer resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(secret, s) {
		msg := fmt.Sprintf(MessageSecretExists, secret.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// TODO: compare existing to spec and reconcile
	return nil
}

func (c *Controller) syncService(key string, s *atlas.DatabaseServer) error {
	// Creates a service with the same name as the server.
	svc, err := c.servicesLister.Services(s.Namespace).Get(s.Name)
	// If an error occurs during Get, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to get service '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseServerStatus(key, s, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	// If the resource doesn't exist, we'll create it
	var spec corev1.Service
	if errors.IsNotFound(err) {
		if s.Spec.DBHost == "" {
			spec = corev1.Service{
				ObjectMeta: c.objMeta(s, "DatabaseServer"),
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"databaseserver": s.Name,
					},
					Ports: []corev1.ServicePort{
						{
							Protocol: "TCP",
							Port:     s.Spec.ServicePort,
						},
					},
				},
			}
			glog.V(4).Info("Creating service with type ClusterIP")
		} else {
			spec = corev1.Service{
				ObjectMeta: c.objMeta(s, "DatabaseServer"),
				Spec: corev1.ServiceSpec{
					Type:         "ExternalName",
					ExternalName: s.Spec.DBHost,
				},
			}
			glog.V(4).Info("Creating service with type ExternalName")
		}
		svc, err = c.kubeclientset.CoreV1().Services(s.Namespace).Create(&spec)
		if err != nil {
			msg := fmt.Sprintf("failed to create service '%s': %s", key, err)
			glog.Error(msg)
			c.updateDatabaseServerStatus(key, s, StateError, msg)
			runtime.HandleError(fmt.Errorf(msg))
			return err
		}
		c.recorder.Event(s, corev1.EventTypeNormal, StateCreated, fmt.Sprintf(MessageServiceCreated, svc.Name))
	}

	// If the Service is not controlled by this DatabaseServer resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(svc, s) {
		msg := fmt.Sprintf(MessageServiceExists, svc.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}
	// TODO: compare existing service to spec and reconcile
	// TODO: recreate service if ExternalName changes
	return nil
}

func (c *Controller) syncPodServer(p plugin.PodPlugin, key string, s *atlas.DatabaseServer) error {
	// Creates a pod with the same name as the server
	pod, err := c.podsLister.Pods(s.Namespace).Get(s.Name)
	// If an error occurs during Get, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to get pod '%s': %s", key, err)
		glog.Error(msg)
		c.updateDatabaseServerStatus(key, s, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		pod, err = c.kubeclientset.CoreV1().Pods(s.Namespace).Create(p.CreatePod(key, s))
		if err != nil {
			msg := fmt.Sprintf("failed to create pod '%s': %s", key, err)
			glog.Error(msg)
			c.updateDatabaseServerStatus(key, s, StateError, msg)
			runtime.HandleError(fmt.Errorf(msg))
			return err
		}
		c.recorder.Event(s, corev1.EventTypeNormal, StateCreated, fmt.Sprintf(MessagePodCreated, pod.Name))
	}

	// If the Pod is not controlled by this DatabaseServer resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(pod, s) {
		msg := fmt.Sprintf(MessagePodExists, pod.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	//TODO: Compare existing pod to spec and reconcile
	/*
		   // Update the pod resource to match the spec
		   diffs := p.DiffPod(s)
		   if diffs != "" {
		       glog.V(4).Infof("DatabaseServer %s needs update: %s", diffs)
		       pod, err = c.kubeclientset.CoreV1().Pods(s.Namespace).Update(p.CreatePod(key, s))
		   }

		   // If an error occurs during Update, we'll requeue the item so we can
		   // attempt processing again later. THis could have been caused by a
		   // temporary network failure, or any other transient reason.
		   if err != nil {
		       return err
		   }
		c.recorder.Event(s, corev1.EventTypeNormal, SuccessSynced, fmt.Sprintf(MessagePodUpdated, pod.Name))
	*/
	return nil
}

func (c *Controller) updateDatabaseServerStatus(key string, s *atlas.DatabaseServer, state, msg string) (*atlas.DatabaseServer, error) {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	copy := s.DeepCopy()
	copy.Status.State = state
	copy.Status.Message = msg
	// UpdateStatus will not allow changes to the Spec of the resource, which is ideal for ensuring
	// nothing other than resource status has been updated.
	_, err := c.atlasclientset.AtlasdbV1alpha1().DatabaseServers(s.Namespace).UpdateStatus(copy)
	if err != nil {
		runtime.HandleError(fmt.Errorf("error updating status to '%s' for database server '%s': %s", state, key, err))
		return s, err
	}
	// we have to pull it back out or our next update will fail. hopefully this is fixed with updateStatus
	return c.serversLister.DatabaseServers(s.Namespace).Get(s.Name)
}
