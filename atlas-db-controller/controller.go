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

	"github.com/golang-migrate/migrate"
	_ "github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/github"

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
	// SuccessSynced is used as part of the Event 'reason' when a resource is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a resource fails
	// to sync due to a resource it should own already existing
	ErrResourceExists = "ErrResourceExists"

	MessageServiceExists  = "Service %q already exists and is not managed by DatabaseServer"
	MessageSecretExists   = "Secret %q already exists and is not managed by DatabaseServer"
	MessagePodExists      = "Pod %q already exists and is not managed by DatabaseServer"
	MessageServerSynced   = "DatabaseServer synced successfully"
	MessageDatabaseSynced = "Database synced successfully"
	MessageSchemaSynced   = "DatabaseSchema synced successfully"

	StateCreating = "Creating"
	StateDeleting = "Deleting"
	StateError    = "Error"
	StatePending  = "Pending"
	StateSuccess  = "Success"
	StateUpdating = "Updating"
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

func (c *Controller) syncSchema(key string) error {
	glog.Infof("Schema key: %v", key)

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return err
	}

	// Get the Schema resource with this namespace/name
	schema, err := c.schemasLister.DatabaseSchemas(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("schema '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}
	glog.V(4).Infof("Schema Spec: %v", schema.Spec)

	// If dsn/dsnFrom is passed in the schema spec consider as override and don't go through database spec
	dsn := schema.Spec.Dsn
	dbName := schema.Spec.Database
	if dsn == "" {
		if schema.Spec.DsnFrom != nil {
			secretName := schema.Spec.DsnFrom.SecretKeyRef.Name
			dsn, err = c.getSecretFromValueSource(schema.Namespace, schema.Spec.DsnFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting to get DSN for schema `%s` from secret `%s`", key, secretName)
					c.updateDatabaseSchemaStatus(key, schema, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid DSN for schema `%s` from secret `%s`", key, secretName)
				c.updateDatabaseSchemaStatus(key, schema, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		} else { // Get the dsn from database created secret
			db, err := c.dbsLister.Databases(namespace).Get(dbName)
			if err != nil {
				schemaStatusMsg = fmt.Sprintf("failed to fetch database info `%s`: %s", dbName, err)
				c.updateDatabaseSchemaStatus(key, schema, StateError, schemaStatusMsg)
				runtime.HandleError(fmt.Errorf(schemaStatusMsg))
				return err
			}

			glog.V(4).Infof("Server type requested: %s", db.Spec.ServerType)
			if db.Spec.ServerType != "postgres" { //  && db.Spec.ServerType != ...
				schemaStatusMsg = fmt.Sprintf("unsupported database server type `%s`, the supported database is postgres", db.Spec.ServerType)
				c.updateDatabaseSchemaStatus(key, schema, StateError, schemaStatusMsg)
				err = fmt.Errorf(schemaStatusMsg)
				runtime.HandleError(err)
				return err
			}

			dsn, err = c.getSecretByName(db.Namespace, "dsn", dbName)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting to get DSN for schema `%s` from secret `%s`", key, dbName)
					c.updateDatabaseSchemaStatus(key, schema, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid DSN for schema `%s` from secret `%s`", key, dbName)
				c.updateDatabaseSchemaStatus(key, schema, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		}
	}

	// Formulate gitURL from either git string or secret provided
	gitURL := schema.Spec.Git
	if gitURL == "" {
		if schema.Spec.GitFrom != nil {
			secretName := schema.Spec.GitFrom.SecretKeyRef.Name
			gitURL, err = c.getSecretFromValueSource(schema.Namespace, schema.Spec.GitFrom)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting to get gitURL for schema `%s` from secret `%s`", key, secretName)
					c.updateDatabaseSchemaStatus(key, schema, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid gitURL for schema `%s` from secret `%s`", key, secretName)
				c.updateDatabaseSchemaStatus(key, schema, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}

		} else {
			msg := fmt.Sprintf("failed to get valid gitURL for schema `%s`", key)
			c.updateDatabaseSchemaStatus(key, schema, StateError, msg)
			err = fmt.Errorf(msg)
			runtime.HandleError(err)
			return err
		}
	}

	mgrt, err := migrate.New(gitURL, dsn)
	if err != nil {
		schemaStatusMsg = fmt.Sprintf("failed to initialize migrate engine: %s", err)
		c.updateDatabaseSchemaStatus(key, schema, StateError, schemaStatusMsg)
		err = fmt.Errorf(schemaStatusMsg)
		runtime.HandleError(err)
		return err
	}
	defer mgrt.Close()

	ver, dirt, err := mgrt.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			glog.Infof("database `%s` has no migration applied", dbName)
			schemaStatusMsg = fmt.Sprintf("database `%s` has no migration applied", dbName)
			c.updateDatabaseSchemaStatus(key, schema, StatePending, schemaStatusMsg)
		} else {
			schemaStatusMsg = fmt.Sprintf("cannot get current database version: %s", err)
			c.updateDatabaseSchemaStatus(key, schema, StateError, schemaStatusMsg)
			err = fmt.Errorf(schemaStatusMsg)
			runtime.HandleError(err)
			return err
		}
	}
	if dirt {
		// TODO we might want to notficate someone about this
		schemaStatusMsg = fmt.Sprintf("database `%s` (%s) is in dirty state (version is %d)", dbName, dsn, ver)
		c.updateDatabaseSchemaStatus(key, schema, StateError, schemaStatusMsg)
		err = fmt.Errorf(schemaStatusMsg)
		runtime.HandleError(err)
		return err
	}
	toVersion := uint(schema.Spec.Version)
	if ver == toVersion {
		glog.Infof("database `%s` has synced version %d", dbName, toVersion)
		schemaStatusMsg = fmt.Sprintf("database `%s` has synced version %d", dbName, toVersion)
		c.updateDatabaseSchemaStatus(key, schema, StateSuccess, schemaStatusMsg)
		return nil
	}

	err = mgrt.Migrate(toVersion)
	if err != nil {
		schemaStatusMsg = fmt.Sprintf("cannot migrate the db %s : %s", dbName, err)
		c.updateDatabaseSchemaStatus(key, schema, StateError, schemaStatusMsg)
		err = fmt.Errorf(schemaStatusMsg)
		runtime.HandleError(err)
		return err
	}

	schemaStatusMsg := fmt.Sprintf("Successfully synced schema '%s'", key)
	schema, err = c.updateDatabaseSchemaStatus(key, schema, StateSuccess, schemaStatusMsg)

	if err != nil {
		runtime.HandleError(err)
		return err
	}

	return nil
}

func (c *Controller) updateDatabaseSchemaStatus(key string, schema *atlas.DatabaseSchema, state, msg string) (*atlas.DatabaseSchema, error) {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	schemaCopy := schema.DeepCopy()
	schemaCopy.Status.State = state
	schemaCopy.Status.Message = msg
	// Until #38113 is merged, we must use Update instead of UpdateStatus to
	// update the Status block of the resource. UpdateStatus will not
	// allow changes to the Spec of the resource, which is ideal for ensuring
	// nothing other than resource status has been updated.

	_, err := c.atlasclientset.AtlasdbV1alpha1().DatabaseSchemas(schema.Namespace).Update(schemaCopy)
	if err != nil {
		runtime.HandleError(fmt.Errorf("error updating status to '%s' for database schema '%s': %s", state, key, err))
		return schema, err
	}
	// we have to pull it back out or our next update will fail. hopefully this is fixed with updateStatus
	return c.schemasLister.DatabaseSchemas(schema.Namespace).Get(schema.Name)
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

	if db.Status.State == "" {
		db, err = c.updateDatabaseStatus(key, db, StatePending, "")
		if err != nil {
			return err
		}
	}

	var p plugin.DatabasePlugin
	var s *atlas.DatabaseServer

	if db.Spec.Server != "" {
		// TODO this implies the same namespace for database and server. Do we want this?
		s, err = c.serversLister.DatabaseServers(namespace).Get(db.Spec.Server)
		if err != nil {
			if errors.IsNotFound(err) {
				msg := fmt.Sprintf("waiting for database server '%s/%s'", namespace, db.Spec.Server)
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
					c.updateDatabaseStatus(key, db, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid DSN for database `%s` from secret `%s`", key, secretName)
				c.updateDatabaseStatus(key, db, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		} else { // Get the dsn with superuser info from database server created secret
			dsn, err = c.getSecretByName(db.Namespace, "dsn", s.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					msg := fmt.Sprintf("waiting to get DSN for database `%s` from secret `%s`", key, s.Name)
					c.updateDatabaseStatus(key, db, StatePending, msg)
					return err
				}
				msg := fmt.Sprintf("failed to get valid DSN for database `%s` from secret `%s`", key, s.Name)
				c.updateDatabaseStatus(key, db, StateError, msg)
				runtime.HandleError(fmt.Errorf(msg))
				return nil
			}
		}
	}

	// Update dsn related to a database which databaseschema will use.
	err = c.syncDatabaseSecret(key, db, s)
	if err != nil {
		msg := fmt.Sprintf("error syncing database secrets '%s': %s", key, err)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return nil
	}

	err = p.SyncDatabase(db, dsn)
	if err != nil {
		msg := fmt.Sprintf("error syncing database '%s': %s", key, err)
		c.updateDatabaseStatus(key, db, StateError, msg)
		runtime.HandleError(fmt.Errorf(msg))
		return err
	}

	msg := fmt.Sprintf("Successfully synced database '%s'", key)
	db, err = c.updateDatabaseStatus(key, db, StateSuccess, msg)
	if err != nil {
		runtime.HandleError(err)
		return err
	}

	//TODO: Log some more events for troubleshoting
	c.recorder.Event(db, corev1.EventTypeNormal, SuccessSynced, MessageDatabaseSynced)
	return nil
}

func (c *Controller) syncDatabaseSecret(key string, db *atlas.Database, dbserver *atlas.DatabaseServer) error {
	if db.Spec.Users == nil {
		glog.V(4).Info(" Database users not provided. Skip database secret creation")
		return nil
	}
	secret, errs := c.secretsLister.Secrets(db.Namespace).Get(db.Name)

	//TODO: check if the secret matches the spec and change it if not
	//this will require additional support from the database plugin

	// If the resource doesn't exist, we'll create it.
	// TODO: creating dsn for admin user alone for now. non-admin users also we should create.
	var err error
	for index, user := range db.Spec.Users {
		passwd := user.Password
		if user.Role == "admin" {
			if user.PasswordFrom != nil {
				passwd, err = c.getSecretFromValueSource(db.Namespace, user.PasswordFrom)
				if err != nil {
					if errors.IsNotFound(err) {
						msg := fmt.Sprintf("waiting for secret or configmap for %s", user.Name)
						c.updateDatabaseStatus(key, db, StatePending, msg)
						return err
					}
				}
				db.Spec.Users[index].Password = passwd
			}
			if errors.IsNotFound(errs) {
				glog.V(4).Info("Database secrets not found. Creating...")
				dsn := server.ActivePlugin(dbserver).Dsn(user.Name, passwd, db, dbserver)
				secret, errs = c.kubeclientset.CoreV1().Secrets(db.Namespace).Create(
					&corev1.Secret{
						ObjectMeta: c.objMeta(db, "Secret"),
						StringData: map[string]string{"dsn": dsn},
					},
				)
			}
		}
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if errs != nil {
		return errs
	}

	// If it is not controlled by this Database resource, we should log
	// a warning to the event recorder and ret
	if !metav1.IsControlledBy(secret, db) {
		msg := fmt.Sprintf(MessageSecretExists, secret.Name)
		c.recorder.Event(dbserver, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// TODO: compare existing to spec and reconcile

	// TODO: Update status
	return nil
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

		dsn := server.ActivePlugin(s).Dsn(su, supw, nil, s)
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

func (c *Controller) objMeta(o metav1.Object, kind string) metav1.ObjectMeta {
	labels := map[string]string{
		"controller": o.GetName(),
	}
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
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		return
	}
	glog.Infof("enqueue database object: %s", object.GetName())
	c.enqueue(obj, c.dbQueue)
}

func (c *Controller) enqueueDatabaseSchema(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		glog.Info("not enqueue schema object")
		return
	}
	glog.Infof("enqueue schema object: %s", object.GetName())
	c.enqueue(obj, c.schemaQueue)
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
