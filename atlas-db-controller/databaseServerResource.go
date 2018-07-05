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

// syncServer compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the DatabaseServer resource
// with the current status of the resource.
func (c *Controller) syncServer(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the DatabaseServer resource with this namespace/name
	s, err := c.serversLister.DatabaseServers(namespace).Get(name)
	if err != nil {
		// The DatabaseServer resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("databaseserver '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	err = c.syncService(key, s)
	if err != nil {
		return err
	}

	p := server.ActivePlugin(s)

	glog.V(4).Infof("DatabaseServer %s has plugin type: %T", key, p)
	err = fmt.Errorf("databaseserver '%s' has an unimplemented plugin type", key)

	if pp, ok := p.(plugin.PodPlugin); ok {
		err = c.syncPodServer(pp, key, s)
	}

	if cp, ok := p.(plugin.CloudPlugin); ok {
		err = cp.SyncCloud(key, s)
	}

	if err != nil {
		return err
	}

	err = c.syncServerSecret(key, s)
	if err != nil {
		return err
	}

	err = c.syncService(key, s)
	if err != nil {
		return err
	}

	//TODO: Add a check to see if the db server is up and running and accessible via the Service and Secret
	// and update the Status and Message appropriately

	return nil
}

func (c *Controller) syncServerSecret(key string, s *atlas.DatabaseServer) error {
	// Creates a secret with the same name as the server. TODO: maybe we should allow different?
	secret, err := c.secretsLister.Secrets(s.Namespace).Get(s.Name)

	//TODO: check if the secret matches the spec and change it if not
	// this will require additional support from the database plugin

	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		glog.V(4).Info("Database Server secrets not found. Creating...")
		su := s.Spec.SuperUser
		if su == "" {
			su, err = c.getSecretFromValueSource(s.Namespace, s.Spec.SuperUserFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := "waiting for secret or configmap for superUser"
					c.updateDatabaseServerStatus(key, s, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("databaseserver '%s' has no valid superUser or source", key)
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
					c.updateDatabaseServerStatus(key, s, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("databaseserver '%s' has no valid superUserPassword or source", key)
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
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If it is not controlled by this DatabaseServer resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(secret, s) {
		msg := fmt.Sprintf(MessageSecretExists, secret.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// TODO: compare existing to spec and reconcile

	// TODO: Update status
	return nil
}

func (c *Controller) syncService(key string, s *atlas.DatabaseServer) error {
	// Creates a service with the same name as the server. TODO: maybe we should allow different?
	svc, err := c.servicesLister.Services(s.Namespace).Get(s.Name)

	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		svc, err = c.kubeclientset.CoreV1().Services(s.Namespace).Create(
			&corev1.Service{
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
			},
		)
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the Service is not controlled by this DatabaseServer resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(svc, s) {
		msg := fmt.Sprintf(MessageServiceExists, svc.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// TODO: compare existing service to spec and reconcile

	// TODO: Update status
	return nil
}

func (c *Controller) syncPodServer(p plugin.PodPlugin, key string, s *atlas.DatabaseServer) error {
	// Creates a pod with the same name as the server
	pod, err := c.podsLister.Pods(s.Namespace).Get(s.Name)

	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		pod, err = c.kubeclientset.CoreV1().Pods(s.Namespace).Create(p.CreatePod(key, s))
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
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
	*/

	// Finally, we update the status block of the DatabaseServer resource to reflect the
	// current state of the world
	s, err = c.updateDatabaseServerStatus(key, s, StateSuccess, "Successfully synced")
	if err != nil {
		return err
	}

	c.recorder.Event(s, corev1.EventTypeNormal, SuccessSynced, MessageServerSynced)
	return nil
}

func (c *Controller) updateDatabaseServerStatus(key string, s *atlas.DatabaseServer, state, msg string) (*atlas.DatabaseServer, error) {
	copy := s.DeepCopy()
	copy.Status.State = state
	copy.Status.Message = msg
	// Until #38113 is merged, we must use Update instead of UpdateStatus to
	// update the Status block of the resource. UpdateStatus will not
	// allow changes to the Spec of the resource, which is ideal for ensuring
	// nothing other than resource status has been updated.
	_, err := c.atlasclientset.AtlasdbV1alpha1().DatabaseServers(s.Namespace).Update(copy)
	if err != nil {
		runtime.HandleError(fmt.Errorf("error updating status to '%s' for database server '%s': %s", state, key, err))
		return s, err
	}
	// we have to pull it back out or our next update will fail. hopefully this is fixed with updateStatus
	return c.serversLister.DatabaseServers(s.Namespace).Get(s.Name)
}

func (c *Controller) enqueueDatabaseServer(obj interface{}) {
	c.enqueue(obj, c.serverQueue)
}
