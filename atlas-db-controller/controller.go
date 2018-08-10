package main

// based on https://github.com/kubernetes/sample-controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	//appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	_ "github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/github"

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	clientset "github.com/infobloxopen/atlas-db/pkg/client/clientset/versioned"
	atlasscheme "github.com/infobloxopen/atlas-db/pkg/client/clientset/versioned/scheme"
	informers "github.com/infobloxopen/atlas-db/pkg/client/informers/externalversions"
	listers "github.com/infobloxopen/atlas-db/pkg/client/listers/db/v1alpha1"
)

const controllerAgentName = "atlas-db-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a resource is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a resource fails
	// to sync due to a resource it should own already existing
	ErrResourceExists = "ErrResourceExists"

	MessageServiceExists  = "Service %q already exists and is not managed by DatabaseServer"
	MessageSecretExists   = "Secret %q already exists and is not managed by DatabaseServer"
	MessagePodExists      = "Pod %q already exists and is not managed by DatabaseServer"
	MessageServerSynced   = "DatabaseServer %q synced successfully"
	MessageDatabaseSynced = "Database %q synced successfully"
	MessageSchemaSynced   = "DatabaseSchema %q synced successfully"

	MessageServiceCreated = "Service %q created successfully"
	MessageSecretCreated  = "Secret %q created successfully"
	MessagePodCreated     = "Pod %q created successfully"

	StateCreated  = "Created"
	StateDeleting = "Deleting"
	StateError    = "Error"
	StatePending  = "Pending"
	StateSuccess  = "Success"
	StateUpdating = "Updating"

	MessageDSNGetFailure = "failed to get DSN for %q from secret %q : %q"
	MessageDSNGetWaiting = "waiting to get DSN for %q from secret %q"
)

var schemaStatusMsg string

// Controller is the controller implementation for DatabaseServer resources
type Controller struct {
	kubeclientset  kubernetes.Interface
	atlasclientset clientset.Interface

	podsLister     corelisters.PodLister
	podsSynced     cache.InformerSynced
	secretsLister  corelisters.SecretLister
	secretsSynced  cache.InformerSynced
	servicesLister corelisters.ServiceLister
	servicesSynced cache.InformerSynced
	serversLister  listers.DatabaseServerLister
	serversSynced  cache.InformerSynced
	dbsLister      listers.DatabaseLister
	dbsSynced      cache.InformerSynced
	schemasLister  listers.DatabaseSchemaLister
	schemasSynced  cache.InformerSynced

	serverQueue workqueue.RateLimitingInterface
	dbQueue     workqueue.RateLimitingInterface
	schemaQueue workqueue.RateLimitingInterface

	recorder record.EventRecorder
}

type syncHandler func(string) error

// NewController returns a new atlas DB controller
func NewController(
	kubeclientset kubernetes.Interface,
	atlasclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	atlasInformerFactory informers.SharedInformerFactory) *Controller {

	// obtain references to shared index informers for Secrets
	secretInformer := kubeInformerFactory.Core().V1().Secrets()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	serverInformer := atlasInformerFactory.Atlasdb().V1alpha1().DatabaseServers()
	dbInformer := atlasInformerFactory.Atlasdb().V1alpha1().Databases()
	schemaInformer := atlasInformerFactory.Atlasdb().V1alpha1().DatabaseSchemas()

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
		secretsLister:  secretInformer.Lister(),
		secretsSynced:  secretInformer.Informer().HasSynced,
		servicesLister: serviceInformer.Lister(),
		servicesSynced: serviceInformer.Informer().HasSynced,
		serversLister:  serverInformer.Lister(),
		serversSynced:  serverInformer.Informer().HasSynced,
		serverQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DatabaseServers"),
		dbsLister:      dbInformer.Lister(),
		dbsSynced:      dbInformer.Informer().HasSynced,
		dbQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Databases"),
		schemasLister:  schemaInformer.Lister(),
		schemasSynced:  schemaInformer.Informer().HasSynced,
		schemaQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DatabaseSchemas"),

		recorder: recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when DatabaseServer resources change
	serverInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDatabaseServer,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDatabaseServer(new)
		},
	})
	// Set up an event handler for when Database resources change
	dbInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDatabase,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDatabase(new)
		},
	})
	// Set up an event handler for when DatabaseSchema resources change
	schemaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDatabaseSchema,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDatabaseSchema(new)
		},
	})
	// Set up an event handlers for resources we might own, and then
	// enqueue them if we own them.
	objInformers := []cache.SharedInformer{
		podInformer.Informer(),
		serviceInformer.Informer(),
		secretInformer.Informer(),
	}
	for _, inf := range objInformers {
		inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleObject,
			UpdateFunc: func(old, new interface{}) {
				n := new.(metav1.Object)
				o := old.(metav1.Object)
				if n.GetResourceVersion() == o.GetResourceVersion() {
					// Periodic resync will send update events for all known objects
					// Two different versions of the same object will always have different RVs.
					return
				}
				controller.handleObject(new)
			},
			DeleteFunc: controller.handleObject,
		})
	}

	return controller
}

// Run will set up the event handlers for types we are interested in, as well // XXX event handlers are set above
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the queues and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.serverQueue.ShutDown()
	defer c.dbQueue.ShutDown()
	defer c.schemaQueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting atlas-db-controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced, c.serversSynced, c.dbsSynced, c.servicesSynced, c.secretsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers...")
	// Launch two workers to process each resource in the group
	for i := 0; i < threadiness; i++ {
		// TODO make sure to do not try run all of three once per second on excatly the same moment
		go wait.Until(c.runServerWorker, time.Second, stopCh)
		go wait.Until(c.runDatabaseWorker, time.Second, stopCh)
		go wait.Until(c.runSchemaWorker, time.Second, stopCh)
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

func (c *Controller) runSchemaWorker() {
	for c.processNextWorkItem(c.schemaQueue, c.syncSchema) {
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

func (c *Controller) getSecretByName(namespace, secretKey, secretName string) (string, error) {
	if secretKey == "" || secretName == "" {
		return "", fmt.Errorf("no valid secretName or secretKey")
	}

	from := &atlas.ValueSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
			Key: secretKey,
		},
	}

	return c.getSecretFromValueSource(namespace, from)
}

func (c *Controller) getSecretFromValueSource(namespace string, from *atlas.ValueSource) (string, error) {
	if from == nil {
		return "", fmt.Errorf("no valid secret value or source")
	}
	return from.Resolve(c.kubeclientset, namespace)
}

func (c *Controller) objMeta(o metav1.Object, kind string) metav1.ObjectMeta {
	labels := o.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["controller"] = o.GetName()
	return metav1.ObjectMeta{
		Name:      o.GetName(),
		Namespace: o.GetNamespace(),
		Labels:    labels,
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(o, schema.GroupVersionKind{
				Group:   atlas.SchemeGroupVersion.Group,
				Version: atlas.SchemeGroupVersion.Version,
				Kind:    kind,
			}),
		},
	}
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

// handleObject will take any resource implementing metav1.Object and attempt
// to find the resource that 'owns' it.
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
	glog.Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		if ownerRef.Kind == "DatabaseServer" {
			s, err := c.serversLister.DatabaseServers(object.GetNamespace()).Get(ownerRef.Name)
			if err != nil {
				glog.V(4).Infof("ignoring orphaned object '%s' of databaseserver '%s'", object.GetSelfLink(), ownerRef.Name)
				return
			}

			c.enqueueDatabaseServer(s)
			return
		}

		if ownerRef.Kind == "Database" {
			d, err := c.dbsLister.Databases(object.GetNamespace()).Get(ownerRef.Name)
			if err != nil {
				glog.V(4).Infof("ignoring orphaned object '%s' of database '%s'", object.GetSelfLink(), ownerRef.Name)
				return
			}

			c.enqueueDatabase(d)
			return
		}

		return
	}
}
