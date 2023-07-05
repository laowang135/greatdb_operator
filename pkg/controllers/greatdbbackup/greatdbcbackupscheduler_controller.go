package greatdbbackup

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"golang.org/x/time/rate"

	v1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"
	greatdbClusterscheme "greatdb-operator/pkg/client/clientset/versioned/scheme"
	greatdbInformer "greatdb-operator/pkg/client/informers/externalversions"
	deps "greatdb-operator/pkg/controllers/dependences"
	resourcesManager "greatdb-operator/pkg/resources/manager"
	dblog "greatdb-operator/pkg/utils/log"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	covev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeinformer "k8s.io/client-go/informers"
	eventv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type GreatDBBackupScheduleController struct {
	Client                      *deps.ClientSet
	Listers                     *deps.Listers
	Recorder                    record.EventRecorder
	Queue                       workqueue.RateLimitingInterface
	GreatDBBackupScheduleSynced cache.InformerSynced
	managers                    *resourcesManager.GreatDBBackupScheduleResourceManager
}

func NewGreatDBBackupScheduleController(
	client *deps.ClientSet,
	listers *deps.Listers,
	greatdbInformer greatdbInformer.SharedInformerFactory,
	kubeLabelInformer kubeinformer.SharedInformerFactory,
) *GreatDBBackupScheduleController {

	utilruntime.Must(greatdbClusterscheme.AddToScheme(scheme.Scheme))

	dblog.Log.V(2).Infof("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	// De logging structured events
	eventBroadcaster.StartStructuredLogging(0)
	//
	eventBroadcaster.StartRecordingToSink(&eventv1.EventSinkImpl{Interface: client.KubeClientset.CoreV1().Events("")})
	// Instantiated event recorder
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, covev1.EventSource{Component: ControllerName})

	manager := resourcesManager.NewGreatDBBackupScheduleResourceManager(client, listers, recorder)

	controller := &GreatDBBackupScheduleController{
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
		GreatDBBackupScheduleSynced: greatdbInformer.Greatdb().V1alpha1().GreatDBBackupSchedules().Informer().HasSynced,
		managers:                    manager,
	}

	dblog.Log.V(1).Info("setting up event handlers")
	greatdbInformer.Greatdb().V1alpha1().GreatDBBackupSchedules().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.addenqueueFn,
		UpdateFunc: func(old, new interface{}) {
			newSchedule := new.(*v1alpha1.GreatDBBackupSchedule)
			oldSchedule := old.(*v1alpha1.GreatDBBackupSchedule)
			// Skip processing if the versions are the same
			if newSchedule.ResourceVersion == oldSchedule.ResourceVersion {
				return
			}
			controller.enqueueFn(new)
		},
		DeleteFunc: controller.enqueueFn,
	})

	return controller

}

func (ctrl *GreatDBBackupScheduleController) Run(threading int, stopCh <-chan struct{}) {
	// Capture crash
	defer utilruntime.HandleCrash()
	// close the queue
	defer ctrl.Queue.ShutDown()

	dblog.Log.Info("starting greatdb-backup-scheduler controller")

	dblog.Log.V(2).Info("waiting for backup-scheduler informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, ctrl.GreatDBBackupScheduleSynced); !ok {
		dblog.Log.Error("failed to wait for cache to sync")
		return
	}

	dblog.Log.V(2).Info("Starting workers")
	for i := 0; i < threading; i++ {
		go wait.Until(ctrl.runWorker, time.Second, stopCh)
	}

	<-stopCh
	dblog.Log.Info("shutting down greatdb-backup-scheduler controller workers")

}

func (ctrl *GreatDBBackupScheduleController) runWorker() {
	for ctrl.processNextWorkItem() {

	}
}

func (ctrl *GreatDBBackupScheduleController) processNextWorkItem() bool {

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
		dblog.Log.Infof("starting sync the %s", key)
		if err := ctrl.Sync(key); err != nil {
			// Synchronization failed, rejoin the queue
			ctrl.Queue.AddRateLimited(obj)
			// Number of records processed
			ctrl.Queue.NumRequeues(obj)
			return fmt.Errorf("error syncing %s : %s, requeuing", key, err.Error())

		}
		dblog.Log.Infof("success sync %s ", key)
		// This object is successfully synchronized, removed from the queue
		ctrl.Queue.Forget(obj)

		return nil
	}(obj)

	if err != nil {
		dblog.Log.Error(err.Error())

	}
	return true

}

// Sync Synchronize the backup state to the desired state
func (ctrl *GreatDBBackupScheduleController) Sync(key string) error {
	dblog.Log.Infof("Start synchronizing greatdb backup %s", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		dblog.Log.Errorf("invalid resource key %s", key)
		return nil
	}
	schedule, err := ctrl.Listers.BackupSchedulerLister.GreatDBBackupSchedules(ns).Get(name)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			dblog.Log.Errorf("Work queue does not have backup %s:%s", ns, name)
			return nil
		}
	}

	if schedule.Spec.Suspend && (schedule.Spec.Suspend == schedule.Status.Suspend) {
		dblog.Log.Infof("backup %s/%s is in manual maintenance mode. In this mode, the operator will not perform status synchronization", schedule.Namespace, schedule.Name)
		return nil
	}

	newSchedule := schedule.DeepCopy()

	// sync backup schedule
	if err = ctrl.syncBackup(newSchedule); err != nil {
		dblog.Log.Reason(err).Errorf("Failed to update backup  schedule%s/%s", newSchedule.Namespace, newSchedule.Name)
		return err
	}

	//  update backup schedule
	var updateBackup *v1alpha1.GreatDBBackupSchedule
	if !reflect.DeepEqual(schedule, newSchedule) {
		updateBackup, err = ctrl.updateGreatDBBackupScheduleScheduler(newSchedule)
		if err != nil {
			return err
		}
		newSchedule.ResourceVersion = updateBackup.ResourceVersion
	}

	if !reflect.DeepEqual(schedule.Status, newSchedule.Status) {
		if err = ctrl.updateGreatDBBackupScheduleSchedulerStatus(newSchedule); err != nil {
			return err
		}
	}

	dblog.Log.Infof("Successfully synchronized backup-schdule %s/%s", newSchedule.Namespace, newSchedule.Name)

	return nil
}

// updateBackup Synchronize the backup schedule state to the desired state
func (ctrl *GreatDBBackupScheduleController) syncBackup(schedule *v1alpha1.GreatDBBackupSchedule) (err error) {

	if !schedule.DeletionTimestamp.IsZero() {

		err = ctrl.updateGreatDBBackupScheduleSchedulerStatus(schedule)
		if err != nil {
			dblog.Log.Error(err.Error())
		}
	}

	// synchronize backup
	if err = ctrl.managers.Scheduler.Sync(schedule); err != nil {
		dblog.Log.Reason(err).Errorf("Failed to synchronize backup schedule %s/%s ", schedule.Namespace, schedule.Name)
		return err
	}

	if err = ctrl.startForegroundDeletion(schedule); err != nil {
		return err
	}
	return nil
}

// updateGreatDBBackupScheduleScheduler Update backup schedule
func (ctrl *GreatDBBackupScheduleController) updateGreatDBBackupScheduleScheduler(backupSchedule *v1alpha1.GreatDBBackupSchedule) (upbackupSchedule *v1alpha1.GreatDBBackupSchedule, err error) {

	oldBackup := backupSchedule.DeepCopy()
	// update fields
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

		upbackupSchedule, err = ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupSchedules(backupSchedule.Namespace).Update(context.TODO(), backupSchedule, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		upSchedule, err1 := ctrl.Listers.BackupSchedulerLister.GreatDBBackupSchedules(backupSchedule.Namespace).Get(backupSchedule.Name)
		if err1 != nil {
			dblog.Log.Errorf("error getting updated greatdb-backup %s/%s from lister,message: %s", backupSchedule.Namespace, backupSchedule.Name, err1.Error())
		} else {
			backupSchedule = upSchedule.DeepCopy()
			backupSchedule.Labels = oldBackup.Labels
			backupSchedule.Finalizers = oldBackup.Finalizers
			backupSchedule.Annotations = oldBackup.Annotations
			backupSchedule.Spec = upSchedule.Spec

		}
		dblog.Log.Errorf("failed to update backup  %s/%s, message: %s", backupSchedule.Namespace, backupSchedule.Name, err.Error())
		return err
	})

	if err != nil {
		dblog.Log.Errorf("failed to update backup %s/%s, message: %s", backupSchedule.Namespace, backupSchedule.Name, err.Error())
	}

	return

}

// UpdateGreatDBClusterStatus Update backup schedule status
func (ctrl *GreatDBBackupScheduleController) updateGreatDBBackupScheduleSchedulerStatus(backupSchedule *v1alpha1.GreatDBBackupSchedule) error {
	oldbackupSchedule := backupSchedule.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupSchedules(backupSchedule.Namespace).UpdateStatus(context.TODO(), backupSchedule, metav1.UpdateOptions{})

		if err == nil {
			return nil
		}
		upBackup, err1 := ctrl.Listers.BackupSchedulerLister.GreatDBBackupSchedules(backupSchedule.Namespace).Get(backupSchedule.Name)
		if err1 != nil {
			dblog.Log.Reason(err).Errorf("failed to lister backup-schdule %s/%s", backupSchedule.Namespace, backupSchedule.Name)
		} else {
			backupSchedule = upBackup.DeepCopy()
			// If there is no change, discard the update
			if reflect.DeepEqual(backupSchedule.Status, oldbackupSchedule.Status) {
				return nil
			}
			backupSchedule.Status = oldbackupSchedule.Status

		}

		dblog.Log.Reason(err).Errorf("failed to update backup schedule %s/%s status", backupSchedule.Namespace, backupSchedule.Name)

		return err
	})

	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to update backup-schedule %s/%s status", backupSchedule.Namespace, backupSchedule.Name)
	}

	return nil

}

func (ctrl GreatDBBackupScheduleController) startForegroundDeletion(backup *v1alpha1.GreatDBBackupSchedule) error {

	if backup.DeletionTimestamp.IsZero() {
		return nil
	}

	if len(backup.Finalizers) == 0 {
		return nil
	}
	patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
	_, err := ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupSchedules(
		backup.Namespace).Patch(context.TODO(), backup.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})

	if err != nil {
		return err
	}
	return nil
}

// addenqueueFn When creating a backup-schedule, add the backup to the queue
func (ctrl *GreatDBBackupScheduleController) addenqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		dblog.Log.Errorf("Invalid object:  %s", err.Error())
		return
	}
	dblog.Log.Infof("Listened to the greatdb-backup-schdule change event, changed object %s", key)

	ctrl.Queue.Add(key)

}

// enqueueFn When updating a backup-schedule, delay adding the backup to the queue
func (ctrl *GreatDBBackupScheduleController) enqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		dblog.Log.Errorf("Invalid object:  %s", err.Error())
		return
	}
	dblog.Log.Infof("Listened to the greatdb-backup-schedule change event, changed object %s", key)
	// Wait five seconds before joining the queue
	ctrl.Queue.AddAfter(key, time.Duration(5*time.Second))
}
