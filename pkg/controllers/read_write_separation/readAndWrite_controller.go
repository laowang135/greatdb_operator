package readwriteseparation

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	gcv1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	queueMgr "greatdb-operator/pkg/controllers/manager"
	"greatdb-operator/pkg/resources"
	resourcesManager "greatdb-operator/pkg/resources/manager"
	dblog "greatdb-operator/pkg/utils/log"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	"k8s.io/client-go/util/workqueue"

	covev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeinformer "k8s.io/client-go/informers"
)

type ReadAndWriteController struct {
	Client   *deps.ClientSet
	Listers  *deps.Listers
	Queue    workqueue.RateLimitingInterface
	managers *resourcesManager.ReadAndWriteManager
	queueMgr *queueMgr.QueueManager
}

// NewReadAndWriteController instantiate an ReadAndWriteController object
func NewReadAndWriteController(
	client *deps.ClientSet,
	listers *deps.Listers,
	kubeLabelInformer kubeinformer.SharedInformerFactory,
) *ReadAndWriteController {

	manager := resourcesManager.NewReadAndWriteManager(client, listers)

	controller := &ReadAndWriteController{
		Client:  client,
		Listers: listers,

		Queue: workqueue.NewNamedRateLimitingQueue( // Mixed speed limit treatment
			workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 1*time.Minute), // Queuing sort
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},  // Token Bucket
			),
			ControllerName,
		),
		managers: manager,
		queueMgr: queueMgr.NewResourcesManager(20, 10),
	}
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

func (ctrl *ReadAndWriteController) Run(threading int, stopCh <-chan struct{}) error {
	// Capture crash
	defer utilruntime.HandleCrash()
	// close the queue
	defer ctrl.Queue.ShutDown()

	dblog.Log.Info("starting read_write_controller")
	for i := 0; i < threading; i++ {
		go wait.Until(ctrl.runWorker, time.Minute*2, stopCh)
	}

	go wait.Until(ctrl.localWatch, time.Second*time.Duration(ctrl.queueMgr.PeriodSeconds), stopCh)

	<-stopCh
	dblog.Log.Info("shutting down read_write_controller controller workers")
	return nil

}

func (ctrl *ReadAndWriteController) runWorker() {
	for ctrl.processNextWorkItem() {

	}
}

func (ctrl *ReadAndWriteController) processNextWorkItem() bool {

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
			utilruntime.HandleError(fmt.Errorf("expected string in queue but got %#v", obj))
			return nil

		}

		if err := ctrl.Sync(key); err != nil {
			// Number of records processed
			num := ctrl.Queue.NumRequeues(obj)
			// Synchronization failed, rejoin the queue
			ctrl.Queue.AddAfter(obj, deps.GetExponentialLevelDelay(num))

			return fmt.Errorf("error syncing %s : %s, requeuing", key, err.Error())

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
func (ctrl *ReadAndWriteController) Sync(key string) error {

	if !ctrl.queueMgr.Add(key, true) {
		return queueMgr.HandlingLimitErr
	}

	if ctrl.queueMgr.Processing(key) {
		return queueMgr.HandlingLimitErr
	}
	defer ctrl.queueMgr.EndOfProcessing(key)

	dblog.Log.Infof("Synchronize read and write services for cluster  %s", key)
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
		return nil
	}

	// update cluster
	if err = ctrl.syncReadAndWrite(cluster); err != nil {
		return fmt.Errorf("synchronize read and write services for cluster  %s", key)
	}

	dblog.Log.Infof("Successfully Synchronize read and write services for cluster  %s", key)

	return nil
}

// updateCluster Synchronize the cluster state to the desired state
func (ctrl *ReadAndWriteController) syncReadAndWrite(cluster *gcv1alpha1.GreatDBPaxos) (err error) {

	if cluster.Status.Phase.Stage() < v1alpha1.GreatDBPaxosReady.Stage() {
		return

	}

	if !cluster.DeletionTimestamp.IsZero() {
		return
	}

	if err := ctrl.managers.Pods.Sync(cluster); err != nil {
		return err
	}

	return nil
}

// enqueueStsFn When the satisfied stateful set changes, obtain the cluster object from the stateful set and join the queue
func (ctrl *ReadAndWriteController) enqueuePodFn(obj interface{}) {

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

func (ctrl *ReadAndWriteController) localWatch() {

	ctrl.queueMgr.Watch(ctrl.Queue)
}
