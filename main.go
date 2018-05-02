package main

import (
	clusterErrors "errors"
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	// cluster-controller pkg libraries
	samsungv1alpha1 "github.com/samsung-cnct/cluster-controller/pkg/apis/clustercontroller/v1alpha1"
	clientset "github.com/samsung-cnct/cluster-controller/pkg/client/clientset/versioned"
	samsungscheme "github.com/samsung-cnct/cluster-controller/pkg/client/clientset/versioned/scheme"
	informers "github.com/samsung-cnct/cluster-controller/pkg/client/informers/externalversions"
	listers "github.com/samsung-cnct/cluster-controller/pkg/client/listers/clustercontroller/v1alpha1"
	"github.com/samsung-cnct/cluster-controller/pkg/signals"

	"github.com/dstorck/gogo"
)

const controllerAgentName = "cluster-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a KrakenCluster is synced
	SuccessSynced = "Synced"
	// MessageResourceSynced is the message used for an Event fired when a KrakenCluster
	// is synced successfully
	MessageResourceSynced = "KrakenCluster synced successfully"
	// CreatedStatus will show the juju status
	CreatedStatus = "CreatedStatus"
	ClusterReady  = "Ready"
	// TODO make MaasEndpoint part of Spec, and JujuBundle an argument
	MaasEndpoint = "http://192.168.2.24/MAAS/api/2.0"
	JujuBundle   = "cs:bundle/kubernetes-core-306"
)

// Controller object
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// samsungclientset is a clientset for our own api group
	samsungclientset clientset.Interface

	krakenclusterLister  listers.KrakenClusterLister
	krakenclustersSynced cache.InformerSynced
	workqueue            workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

var (
	masterURL  string
	kubeconfig string
)

func (c *Controller) run(threadiness int, stopCh <-chan struct{}) error {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	glog.Info("Starting cluster-controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.krakenclustersSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	glog.Info("Started workers")
	// wait until we're told to stop
	<-stopCh
	glog.Info("Shutting down cluster-controller")

	return nil
}

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
		// Foo resource to be synced.
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

func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the KrakenCluster resource with this namespace/name
	kc, err := c.krakenclusterLister.KrakenClusters(namespace).Get(name)
	if err != nil {
		// The KrakenCluster resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("krakencluster '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	// Calls to Business logic go here
	clusterName := kc.Spec.Cluster.ClusterName
	glog.Infof("Received KrakenCluster object for clusterName %s", clusterName)
	switch kc.Status.State {
	case samsungv1alpha1.Unknown:
		glog.Infof("Processing Unknown state for %s", clusterName)
		// process create
		err = c.createCluster(kc)
		if err != nil {
			return err
		}
		// add Finalizer so the resource won't be deleted immediately on delete kc
		kc.SetFinalizers([]string{"samsung.cnct.com/finalizer"})

		err = c.updateKrakenClusterStatus(kc, samsungv1alpha1.Creating, nil)
		if err != nil {
			return err
		}
	case samsungv1alpha1.Creating:
		glog.Infof("Processing Creating state for %s", clusterName)
		// check for delete
		if kc.DeletionTimestamp != nil {
			glog.Infof("Processing Delete for %s", clusterName)
			c.deleteCluster(kc)
			err = c.updateKrakenClusterStatus(kc, samsungv1alpha1.Deleting, nil)
			if err != nil {
				return err
			}
		} else {
			if c.isClusterReady(kc) {
				cluster := gogo.Juju{Name: string(kc.UID)}
				karray, err := cluster.GetKubeConfig()
				kubeconf := ""
				if err != nil {
					glog.Error(err)
				} else {
					kubeconf = string(karray)
				}
				err = c.updateKrakenClusterStatus(kc, samsungv1alpha1.Created, &kubeconf)
				if err != nil {
					return err
				}
				// TODO add juju status to an event

			}
		}
	case samsungv1alpha1.Created:
		glog.Infof("Processing Created state for '%s'", clusterName)
		// check for delete
		if kc.DeletionTimestamp != nil {
			glog.Infof("Processing Delete for %s", clusterName)
			c.deleteCluster(kc)
			err = c.updateKrakenClusterStatus(kc, samsungv1alpha1.Deleting, nil)
			if err != nil {
				return err
			}
		}
	case samsungv1alpha1.Deleting:
		glog.Infof("Processing Deleting state for '%s'", clusterName)
		if c.isDestroyComplete(kc) {
			err = c.updateKrakenClusterStatus(kc, samsungv1alpha1.Deleted, nil)
			if err != nil {
				return err
			}
		}
	case samsungv1alpha1.Deleted:
		glog.Infof("Processing Deleted state for %s", clusterName)
		// remove the Finalizer field so the resource can be deleted
		kc.SetFinalizers(nil)
		err = c.updateKrakenClusterStatus(kc, samsungv1alpha1.Deleted, nil)
		if err != nil {
			return err
		}
	}

	c.recorder.Event(kc, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateKrakenClusterStatus(kc *samsungv1alpha1.KrakenCluster, state samsungv1alpha1.KrakenClusterState, kubeconf *string) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	kcCopy := kc.DeepCopy()
	kcCopy.Status.State = state
	if state != samsungv1alpha1.Created {
		kcCopy.Status.Status = string(state)
	} else {
		kcCopy.Status.Status = ClusterReady
	}

	if kubeconf != nil {
		kcCopy.Status.Kubeconfig = *kubeconf
	}
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the Foo resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.samsungclientset.SamsungV1alpha1().KrakenClusters(kc.Namespace).Update(kcCopy)
	return err
}

func (c *Controller) isClusterReady(kc *samsungv1alpha1.KrakenCluster) bool {
	cluster := gogo.Juju{Name: string(kc.UID)}
	ok, err := cluster.ClusterReady()
	if err != nil {
		glog.Warningf("Cluster Ready Check returned error: %s", err)
	}
	return ok
}

func (c *Controller) isDestroyComplete(kc *samsungv1alpha1.KrakenCluster) bool {
	cluster := gogo.Juju{Name: string(kc.UID)}
	ok, err := cluster.DestroyComplete()
	if err != nil {
		glog.Warningf("Cluster Destroy Check returned error: %s", err)
	}
	return ok
}

func (c *Controller) createCluster(kc *samsungv1alpha1.KrakenCluster) error {
	// TODO remove hardcoding for MaaS.  Also support AWS. Use apiMachinary errors.
	if kc.Spec.CloudProvider.Name != samsungv1alpha1.MaasProvider {
		return clusterErrors.New("Invalid Cloudprovider.  Valid providers are: maas")
	}

	cluster := gogo.Juju{
		Name:   string(kc.UID),
		Kind:   gogo.Maas,
		Bundle: JujuBundle,
		MaasCl: gogo.MaasCloud{
			Endpoint: MaasEndpoint,
		},
		MaasCr: gogo.MaasCredentials{
			Username:  kc.Spec.CloudProvider.Credentials.Username,
			MaasOauth: kc.Spec.CloudProvider.Credentials.Password,
		},
	}

	// TODO this blocks for a few minutes creating the juju controller,
	// but the juju controller is needed for other commands.  Use a Go
	// routine and update state when this is ready?
	cluster.Spinup()
	return nil
}

func (c *Controller) deleteCluster(kc *samsungv1alpha1.KrakenCluster) {
	// TODO remove hardcoding for Maas
	cluster := gogo.Juju{
		Name: string(kc.UID),
		Kind: gogo.Maas,
	}
	cluster.DestroyCluster()
}

// enqueueKrakenCluster takes a KrakenCluster resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than KrakenCluster.
func (c *Controller) enqueueKrakenCluster(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// return rest config, if path not specified assume in cluster config
func getClientConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	}
	return rest.InClusterConfig()
}

// create new controller
func newController(
	kubeclientset kubernetes.Interface,
	samsungclientset clientset.Interface,
	samsungInformerFactory informers.SharedInformerFactory,
) *Controller {
	// obtain references to the shared index informer for KrakenCluster types
	krakenclusterInformer := samsungInformerFactory.Samsung().V1alpha1().KrakenClusters()

	// create event broadcaster so events can be logged for krakencluster types
	samsungscheme.AddToScheme(scheme.Scheme)
	glog.Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	c := &Controller{
		kubeclientset:        kubeclientset,
		samsungclientset:     samsungclientset,
		krakenclusterLister:  krakenclusterInformer.Lister(),
		krakenclustersSynced: krakenclusterInformer.Informer().HasSynced,
		workqueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "KrakenClusters"),
		recorder:             recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when KrakenCluster resources change
	krakenclusterInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueueKrakenCluster,
		UpdateFunc: func(old, new interface{}) {
			glog.Info("Update called")
			c.enqueueKrakenCluster(new)
		},
	})

	return c
}

func main() {
	// pass kubeconfig like: -kubeconfig=$HOME/.kube/config
	// incluster config: -kubeconfig=""
	kubeconf := flag.String("kubeconfig", "admin.conf", "Path to a kube config. Only required if out-of-cluster.")
	flag.Parse()

	config, err := getClientConfig(*kubeconf)
	if err != nil {
		glog.Fatalf("Error loading cluster config: %s", err.Error())
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	samsungClient, err := clientset.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}
	glog.Info("Constructing informer factory")
	samsungInformerFactory := informers.NewSharedInformerFactory(samsungClient, time.Second*30)

	glog.Info("Constructing controller")
	controller := newController(kubeClient, samsungClient, samsungInformerFactory)
	go samsungInformerFactory.Start(stopCh)

	if err = controller.run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}
