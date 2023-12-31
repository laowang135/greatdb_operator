package greatdbpaxos

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"golang.org/x/time/rate"

	gcv1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"
	greatdbClusterscheme "greatdb-operator/pkg/client/clientset/versioned/scheme"
	greatdbInformer "greatdb-operator/pkg/client/informers/externalversions"
	deps "greatdb-operator/pkg/controllers/dependences"
	queueMgr "greatdb-operator/pkg/controllers/manager"
	"greatdb-operator/pkg/resources"
	resourcesManager "greatdb-operator/pkg/resources/manager"
	dblog "greatdb-operator/pkg/utils/log"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	covev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeinformer "k8s.io/client-go/informers"
	eventv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type GreatDBClusterController struct {
	Client               *deps.ClientSet
	Listers              *deps.Listers
	Recorder             record.EventRecorder
	Queue                workqueue.RateLimitingInterface
	GreatdbClusterSynced cache.InformerSynced
	managers             *resourcesManager.GreatDBPaxosResourceManagers
	queueMgr             *queueMgr.QueueManager
}

// NewGreatDBClusterController instantiate an GreatDBClusterController object
func NewGreatDBClusterController(
	client *deps.ClientSet,
	listers *deps.Listers,
	greatdbInformer greatdbInformer.SharedInformerFactory,
	kubeLabelInformer kubeinformer.SharedInformerFactory,
) *GreatDBClusterController {

	utilruntime.Must(greatdbClusterscheme.AddToScheme(scheme.Scheme))

	dblog.Log.V(2).Infof("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	// De logging structured events
	eventBroadcaster.StartStructuredLogging(0)
	//
	eventBroadcaster.StartRecordingToSink(&eventv1.EventSinkImpl{Interface: client.KubeClientset.CoreV1().Events("")})
	// Instantiated event recorder
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, covev1.EventSource{Component: ControllerName})

	manager := resourcesManager.NewGreatDBPaxosResourceManagers(client, listers, recorder)

	controller := &GreatDBClusterController{
		Client:   client,
		Listers:  listers,
		Recorder: recorder,
		Queue: workqueue.NewNamedRateLimitingQueue( // Mixed speed limit treatment
			workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 1*time.Minute), // Queuing sort
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},  // Token Bucket
			),
			ControllerName,
		),
		GreatdbClusterSynced: greatdbInformer.Greatdb().V1alpha1().GreatDBPaxoses().Informer().HasSynced,
		managers:             manager,
		queueMgr:             queueMgr.NewResourcesManager(30, 6),
	}

	dblog.Log.V(1).Info("setting up event handlers")
	greatdbInformer.Greatdb().V1alpha1().GreatDBPaxoses().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.addenqueueFn,
		UpdateFunc: func(old, new interface{}) {
			newCluster := new.(*gcv1alpha1.GreatDBPaxos)
			oldCluster := old.(*gcv1alpha1.GreatDBPaxos)
			// Skip processing if the versions are the same
			if newCluster.ResourceVersion == oldCluster.ResourceVersion {
				return
			}
			controller.enqueueFn(new)
		},
		DeleteFunc: controller.enqueueFn,
	})

	// Add the event handling hook of stateful set
	dblog.Log.V(2).Info("Add the event handling hook of pods")
	kubeLabelInformer.Core().V1().Pods().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueuePodFn,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*covev1.Pod)
			oldPod := old.(*covev1.Pod)

			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			controller.enqueuePodFn(new)
		},
		DeleteFunc: controller.enqueuePodFn,
	})

	return controller

}

func (ctrl *GreatDBClusterController) Run(threading int, stopCh <-chan struct{}) {
	// Capture crash
	defer utilruntime.HandleCrash()
	// close the queue
	defer ctrl.Queue.ShutDown()

	dblog.Log.Info("starting greatdb-cluster controller")

	dblog.Log.V(2).Info("waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, ctrl.GreatdbClusterSynced); !ok {
		dblog.Log.Error("failed to wait for cache to sync")
		return
	}

	dblog.Log.V(2).Info("Starting workers")
	for i := 0; i < threading; i++ {
		go wait.Until(ctrl.runWorker, time.Second*30, stopCh)
	}

	go wait.Until(ctrl.localWatch, time.Second*time.Duration(ctrl.queueMgr.PeriodSeconds), stopCh)

	<-stopCh
	dblog.Log.Info("shutting down greatdb-cluster controller workers")

}

func (ctrl *GreatDBClusterController) runWorker() {
	for ctrl.processNextWorkItem() {

	}
}

func (ctrl *GreatDBClusterController) processNextWorkItem() bool {

	obj, shutdown := ctrl.Queue.Get()
	// Exit if the queue is close
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {

		defer ctrl.Queue.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			ctrl.Queue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in queue but got %v", obj))
			return nil

		}

		if err := ctrl.Sync(key); err != nil {
			// Number of records processed
			num := ctrl.Queue.NumRequeues(obj)
			// Synchronization failed, rejoin the queue
			ctrl.Queue.AddAfter(obj, deps.GetExponentialLevelDelay(num))

			return fmt.Errorf("error syncing %s: message: %s ", key, err.Error())

		}
		// This object is successfully synchronized, removed from the queue
		ctrl.Queue.Forget(obj)

		return nil
	}(obj)

	if err != nil {
		dblog.Log.Error(err.Error())
	}

	return true

}

// Sync Synchronize the cluster state to the desired state
func (ctrl *GreatDBClusterController) Sync(key string) error {

	if !ctrl.queueMgr.Add(key, true) {
		dblog.Log.Infof("instance %s has exceeded the maximum processing times per minute and is currently under speed limit", key)
		return queueMgr.HandlingLimitErr
	}

	if ctrl.queueMgr.Processing(key) {
		dblog.Log.Infof("instance %s is currently being processed", key)
		return queueMgr.SkipErr
	}
	defer ctrl.queueMgr.EndOfProcessing(key)

	dblog.Log.Infof("Start synchronizing greatdb cluster %s", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		dblog.Log.Errorf("invalid resource key %s", key)
		return nil
	}
	cluster, err := ctrl.Listers.PaxosLister.GreatDBPaxoses(ns).Get(name)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			dblog.Log.Errorf("Work queue does not have cluster %s", key)
			ctrl.queueMgr.Delete(key)
			return nil
		}
	}

	if cluster.Spec.MaintenanceMode {
		dblog.Log.Infof("Cluster %s is in manual maintenance mode. In this mode, the operator will not perform status synchronization", key)
		return nil
	}

	newCluster := cluster.DeepCopy()

	// Set Default Configuration
	if ctrl.setDefault(newCluster) {
		_, err = ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBPaxoses(cluster.Namespace).Update(context.TODO(), newCluster, metav1.UpdateOptions{})

		if err != nil {
			return err
		}
		dblog.Log.Info("Updating cluster defaults succeeded")
	}

	// update cluster
	if err = ctrl.syncCluster(newCluster); err != nil {
		return err
	}

	//  update cluster
	var updateCluster *gcv1alpha1.GreatDBPaxos
	if !reflect.DeepEqual(cluster, newCluster) {
		updateCluster, err = ctrl.updateGreatDBCluster(newCluster)
		if err != nil {
			return err
		}
		newCluster.ResourceVersion = updateCluster.ResourceVersion
	}

	if !reflect.DeepEqual(cluster.Status, newCluster.Status) {
		if err = ctrl.updateGreatDBClusterStatus(newCluster); err != nil {
			return err
		}
	}

	dblog.Log.Infof("Successfully synchronized cluster %s", key)

	return nil
}

func (ctrl GreatDBClusterController) setDefault(cluster *gcv1alpha1.GreatDBPaxos) bool {
	return SetDefaultFields(cluster)
}

// updateCluster Synchronize the cluster state to the desired state
func (ctrl *GreatDBClusterController) syncCluster(cluster *gcv1alpha1.GreatDBPaxos) (err error) {

	// synchronize cluster secret
	if err = ctrl.managers.Secret.Sync(cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize secret, message: %s ", err.Error())
		return err
	}

	// Synchronize service
	if err = ctrl.managers.Service.Sync(cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize service, message: %s ", err.Error())
		return err
	}

	// Synchronize configmap
	if err = ctrl.managers.ConfigMap.Sync(cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize configmap, message: %s ", err.Error())
		return err
	}

	// Synchronize GreatDB
	if err = ctrl.managers.GreatDB.Sync(cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize GreatDB , message: %s ", err.Error())
		return err
	}

	if err = ctrl.managers.Dashboard.Sync(cluster); err != nil {
		return err
	}

	if err = ctrl.startForegroundDeletion(cluster); err != nil {
		return err
	}
	return nil
}

// UpdateGreatDBClusterStatus Update cluster
func (ctrl *GreatDBClusterController) updateGreatDBCluster(cluster *gcv1alpha1.GreatDBPaxos) (updateCluster *gcv1alpha1.GreatDBPaxos, err error) {

	oldcluster := cluster.DeepCopy()
	// update fields
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

		updateCluster, err = ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBPaxoses(cluster.Namespace).Update(context.TODO(), cluster, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}

		upcluster, err1 := ctrl.Listers.PaxosLister.GreatDBPaxoses(cluster.Namespace).Get(cluster.Name)
		if err1 != nil {
			dblog.Log.Errorf("error getting updated greatdb-cluster %s/%s from lister,message: %s", cluster.Namespace, cluster.Name, err1.Error())
			return err1
		} else {
			cluster = upcluster.DeepCopy()
			// In addition to setting default values, internal processes should not modify specs
			cluster.Labels = resources.MegerLabels(cluster.Labels, oldcluster.Labels)
			cluster.Annotations = resources.MegerAnnotation(cluster.Annotations, oldcluster.Annotations)
			cluster.Finalizers = oldcluster.Finalizers
		}
		return err
	})

	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to update cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	return

}

// UpdateGreatDBClusterStatus Update cluster status
func (ctrl *GreatDBClusterController) updateGreatDBClusterStatus(cluster *gcv1alpha1.GreatDBPaxos) error {
	oldcluster := cluster.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBPaxoses(cluster.Namespace).UpdateStatus(context.TODO(), cluster, metav1.UpdateOptions{})

		if err == nil {
			return nil
		}
		upcluster, err1 := ctrl.Listers.PaxosLister.GreatDBPaxoses(cluster.Namespace).Get(cluster.Name)
		if err1 != nil {
			dblog.Log.Errorf("error getting updated greatdb-cluster %s/%s from lister,message: %s", cluster.Namespace, cluster.Name, err1.Error())
		} else {
			cluster = upcluster.DeepCopy()
			// If there is no change, discard the update
			if reflect.DeepEqual(cluster.Status, oldcluster.Status) {
				return nil
			}
			cluster.Status = oldcluster.Status

		}

		dblog.Log.Errorf("failed to update cluster %s/%s status, message: %s", cluster.Namespace, cluster.Name, err.Error())

		return err
	})

	if err != nil {
		dblog.Log.Errorf("failed to update cluster %s/%s status, message: %s", cluster.Namespace, cluster.Name, err.Error())
	}

	return nil

}

func (ctrl GreatDBClusterController) startForegroundDeletion(cluster *gcv1alpha1.GreatDBPaxos) error {

	if cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	if len(cluster.Finalizers) == 0 {
		return nil
	}
	patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
	_, err := ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBPaxoses(
		cluster.Namespace).Patch(context.TODO(), cluster.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})

	if err != nil {
		return err
	}
	return nil
}

// addenqueueFn When creating a cluster, add the cluster to the queue
func (ctrl *GreatDBClusterController) addenqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		dblog.Log.Errorf("Invalid object:  %s", err.Error())
		return
	}
	dblog.Log.Infof("Listened to the greatdb-cluster add event, changed object %s", key)

	ctrl.Queue.Add(key)

}

// enqueueFn When updating a cluster, delay adding the cluster to the queue
func (ctrl *GreatDBClusterController) enqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		dblog.Log.Errorf("Invalid object:  %s", err.Error())
		return
	}
	dblog.Log.Infof("Listened to the greatdb-cluster update event, update object %s", key)
	// Wait five seconds before joining the queue
	ctrl.Queue.AddAfter(key, time.Duration(5*time.Second))
}

// enqueueStsFn When the satisfied stateful set changes, obtain the cluster object from the stateful set and join the queue
func (ctrl *GreatDBClusterController) enqueuePodFn(obj interface{}) {

	pod, ok := obj.(*v1.Pod)
	if !ok {
		return
	}

	clusterName, ok := pod.Labels[resources.AppKubeInstanceLabelKey]
	if !ok {
		return
	}

	_, err := ctrl.Listers.PaxosLister.GreatDBPaxoses(pod.Namespace).Get(clusterName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			dblog.Log.V(3).Infof("The greatdb cluster instance %s/%s from pods %s/%s does not exist", pod.Namespace, clusterName, pod.Namespace, pod.Name)
			return
		}
		dblog.Log.Error(err.Error())
		return
	}

	key := fmt.Sprintf("%s/%s", pod.Namespace, clusterName)

	// Wait five seconds before joining the queue
	ctrl.Queue.AddAfter(key, time.Duration(5*time.Second))
}

func (ctrl *GreatDBClusterController) localWatch() {

	ctrl.queueMgr.RWLock.Lock()
	defer ctrl.queueMgr.RWLock.Unlock()
	for key := range ctrl.queueMgr.Resources {
		if ctrl.queueMgr.Add(key, false) {
			ctrl.Queue.Add(key)
		}
	}

}
