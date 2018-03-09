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

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	clientset "github.com/infobloxopen/atlas-db/pkg/client/clientset/versioned"
	atlasscheme "github.com/infobloxopen/atlas-db/pkg/client/clientset/versioned/scheme"
	informers "github.com/infobloxopen/atlas-db/pkg/client/informers/externalversions"
	listers "github.com/infobloxopen/atlas-db/pkg/client/listers/db/v1alpha1"

	"github.com/infobloxopen/atlas-db/pkg/server"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
)

const controllerAgentName = "atlas-db-controller"

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
	kubeclientset kubernetes.Interface
	atlasclientset clientset.Interface

	podsLister    corelisters.PodLister
	podsSynced    cache.InformerSynced
	serversLister listers.DatabaseServerLister
	serversSynced cache.InformerSynced
	dbsLister     listers.DatabaseLister
	dbsSynced     cache.InformerSynced

	serverQueue workqueue.RateLimitingInterface
	dbQueue workqueue.RateLimitingInterface

	recorder record.EventRecorder
}

type syncHandler func(string) error

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
	dbInformer := atlasInformerFactory.Atlasdb().V1alpha1().Databases()

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
		serverQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DatabaseServers"),
		dbsLister:      dbInformer.Lister(),
		dbsSynced:      dbInformer.Informer().HasSynced,
		dbQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Databases"),
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
	// Set up an event handler for when DatabaseServer resources change
	dbInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDatabase,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDatabase(new)
		},
	})
	// Set up an event handlers for resources we might own, and then
	// enqueue them if we own them.
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			n := new.(*corev1.Pod)
			o := old.(*corev1.Pod)
			if n.ResourceVersion == o.ResourceVersion {
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
// is closed, at which point it will shutdown the queues and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.serverQueue.ShutDown()
	defer c.dbQueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting atlas-db-controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced, c.serversSynced, c.dbsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process DatabaseServer resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runServerWorker, time.Second, stopCh)
		go wait.Until(c.runDatabaseWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// run*Worker are long-running functions that will continually call the
// processNextWorkItem function in order to read and process a message on the
// queue.
func (c *Controller) runServerWorker() {
	for c.processNextWorkItem(c.serverQueue, c.syncServer) {
	}
}

func (c *Controller) runDatabaseWorker() {
	for c.processNextWorkItem(c.dbQueue, c.syncDatabase) {
	}
}

// processNextWorkItem will read a single work item off the queue and
// attempt to process it, by calling syncServer.
func (c *Controller) processNextWorkItem(q workqueue.RateLimitingInterface, syncObject syncHandler) bool {
	obj, shutdown := q.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer q.Done.
	err := func(obj interface{}) error {
		defer q.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			q.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in work queue but got %#v", obj))
			return nil
		}
		// Run the sync handler, passing it the namespace/name string of the resource to be synced.
		if err := syncObject(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		q.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncDatabase(key string) error {
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

	glog.V(4).Infof("Database %s: %v", key, db)

	return nil
}

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
	if pp, ok := p.(plugin.PodPlugin); ok {
		return c.syncPodServer(pp, key, s)
	}

	if cp, ok := p.(plugin.CloudPlugin); ok {
		return cp.SyncCloud(key, s)
	}

	return fmt.Errorf("databaseserver '%s' has an unimplemented plugin type", key)
}

func (c *Controller) syncService(key string, s *atlas.DatabaseServer) error {
	return nil
}

func (c *Controller) syncPodServer(p plugin.PodPlugin, key string, s *atlas.DatabaseServer) error {
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
		msg := fmt.Sprintf(MessageResourceExists, pod.Name)
		c.recorder.Event(s, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

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
	err = c.updateDatabaseServerStatus(s, pod)
	if err != nil {
		return err
	}

	c.recorder.Event(s, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateDatabaseServerStatus(s *atlas.DatabaseServer, pod *corev1.Pod) error {
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

func (c *Controller) enqueue(obj interface{}, q workqueue.RateLimitingInterface) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	q.AddRateLimited(key)
}

func (c *Controller) enqueueDatabaseServer(obj interface{}) {
	c.enqueue(obj, c.serverQueue)
}

func (c *Controller) enqueueDatabase(obj interface{}) {
	c.enqueue(obj, c.dbQueue)
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
