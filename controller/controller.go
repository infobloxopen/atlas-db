package main

// based on https://github.com/kubernetes/sample-controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	//appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	//appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	atlasdbv1alpha1 "github.com/infobloxopen/atlas/pkg/apis/atlasdb/v1alpha1"
	clientset "github.com/infobloxopen/atlas/pkg/client/clientset/versioned"
	atlasscheme "github.com/infobloxopen/atlas/pkg/client/clientset/versioned/scheme"
	informers "github.com/infobloxopen/atlas/pkg/client/informers/externalversions"
	listers "github.com/infobloxopen/atlas/pkg/client/listers/atlasdb/v1alpha1"

	"github.com/infobloxopen/atlas/pkg/db/server"
)

const controllerAgentName = "atlas-dbserver-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a DatabaseServer is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a DatabaseServer fails
	// to sync due to a Pod of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Pod already existing
	MessageResourceExists = "Resource %q already exists and is not managed by DatabaseServer"
	// MessageResourceSynced is the message used for an Event fired when a DatabaseServer
	// is synced successfully
	MessageResourceSynced = "DatabaseServer synced successfully"
)

// Controller is the controller implementation for DatabaseServer resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// atlasclientset is a clientset for our own API group
	atlasclientset clientset.Interface

	podsLister    corelisters.PodLister
	podsSynced    cache.InformerSynced
	serversLister listers.DatabaseServerLister
	serversSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new atlas DB controller
func NewController(
	kubeclientset kubernetes.Interface,
	atlasclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	atlasInformerFactory informers.SharedInformerFactory) *Controller {

	// obtain references to shared index informers for Secrets and PVCs
	//secretsInformer := kubeInformerFactory.Apps().V1().Secrets()
	//pvcInformer := kubeInformerFactory.Apps().V1().PersistentVolumeClaims()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	serverInformer := atlasInformerFactory.Atlasdb().V1alpha1().DatabaseServers()

	// Create event broadcaster
	// Add atlas-db-controller types to the default Kubernetes Scheme so Events can be
	// logged for atlas-db-controller types.
	atlasscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:  kubeclientset,
		atlasclientset: atlasclientset,
		podsLister:     podInformer.Lister(),
		podsSynced:     podInformer.Informer().HasSynced,
		serversLister:  serverInformer.Lister(),
		serversSynced:  serverInformer.Informer().HasSynced,
		workqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DatabaseServers"),
		recorder:       recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when DatabaseServer resources change
	serverInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDatabaseServer,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDatabaseServer(new)
		},
	})
	// Set up an event handler for when Pod resources change. This
	// handler will lookup the owner of the given Pod, and if it is
	// owned by a DatabaseServer resource will enqueue that DatabaseServer resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Pod resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*corev1.Pod)
			oldDepl := old.(*corev1.Pod)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Pods.
				// Two different versions of the same Pod will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting DatabaseServer controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced, c.serversSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process DatabaseServer resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// DatabaseServer resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the DatabaseServer resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
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

	podName := s.Name
	if podName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		runtime.HandleError(fmt.Errorf("%s: pod name must be specified", key))
		return nil
	}

	// Get the pod with the name specified in Foo.spec
	pod, err := c.podsLister.Pods(s.Namespace).Get(podName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		pod, err = c.kubeclientset.CoreV1().Pods(s.Namespace).Create(server.NewPod(s))
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
		msg := fmt.Sprintf(MessageResourceExists, pod.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	/*
	// If this number of the replicas on the DatabaseServer resource is specified, and the
	// number does not equal the current desired replicas on the Pod, we
	// should update the Pod resource.
	if s.Spec.Replicas != nil && *s.Spec.Replicas != *pod.Spec.Replicas {
		glog.V(4).Infof("DatabaseServer %s replicas: %d, pod replicas: %d", name, *s.Spec.Replicas, *pod.Spec.Replicas)
		pod, err = c.kubeclientset.CoreV1().Pods(s.Namespace).Update(server.NewPod(s))
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
	err = c.updateDatabaseServerStatus(s, pod)
	if err != nil {
		return err
	}

	c.recorder.Event(s, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateDatabaseServerStatus(s *atlasdbv1alpha1.DatabaseServer, pod *corev1.Pod) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	serverCopy := s.DeepCopy()
	//serverCopy.Status.AvailableReplicas = pod.Status.AvailableReplicas
	// Until #38113 is merged, we must use Update instead of UpdateStatus to
	// update the Status block of the DatabaseServer resource. UpdateStatus will not
	// allow changes to the Spec of the resource, which is ideal for ensuring
	// nothing other than resource status has been updated.
	_, err := c.atlasclientset.AtlasdbV1alpha1().DatabaseServers(s.Namespace).Update(serverCopy)
	return err
}

// enqueueDatabaseServer takes a DatabaseServer resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than DatabaseServer.
func (c *Controller) enqueueDatabaseServer(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the DatabaseServer resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that DatabaseServer resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	glog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a DatabaseServer, we should not do anything more
		// with it.
		if ownerRef.Kind != "DatabaseServer" {
			return
		}

		s, err := c.serversLister.DatabaseServers(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			glog.V(4).Infof("ignoring orphaned object '%s' of databaseserver '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueDatabaseServer(s)
		return
	}
}
