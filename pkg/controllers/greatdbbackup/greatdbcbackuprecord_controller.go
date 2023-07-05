package greatdbbackup

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
	"greatdb-operator/pkg/resources"
	resourcesManager "greatdb-operator/pkg/resources/manager"
	dblog "greatdb-operator/pkg/utils/log"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	batchv1 "k8s.io/api/batch/v1"
	covev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeinformer "k8s.io/client-go/informers"
	eventv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type GreatDbBackupRecordController struct {
	Client              *deps.ClientSet
	Listers             *deps.Listers
	Recorder            record.EventRecorder
	Queue               workqueue.RateLimitingInterface
	GreatdbBackupSynced cache.InformerSynced
	managers            *resourcesManager.GreatDBBackupRecordResourceManager
}

// NewGreatDbBackupRecordController instantiate an GreatDbBackupRecordController object
func NewGreatDbBackupRecordController(
	client *deps.ClientSet,
	listers *deps.Listers,
	greatdbInformer greatdbInformer.SharedInformerFactory,
	kubeLabelInformer kubeinformer.SharedInformerFactory,
) *GreatDbBackupRecordController {

	utilruntime.Must(greatdbClusterscheme.AddToScheme(scheme.Scheme))

	dblog.Log.V(2).Infof("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	// De logging structured events
	eventBroadcaster.StartStructuredLogging(0)
	//
	eventBroadcaster.StartRecordingToSink(&eventv1.EventSinkImpl{Interface: client.KubeClientset.CoreV1().Events("")})
	// Instantiated event recorder
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, covev1.EventSource{Component: ControllerName})

	manager := resourcesManager.NewGreatDBBackupRecordResourceManager(client, listers, recorder)

	controller := &GreatDbBackupRecordController{
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
		GreatdbBackupSynced: greatdbInformer.Greatdb().V1alpha1().GreatDBBackupRecords().Informer().HasSynced,
		managers:            manager,
	}

	dblog.Log.V(1).Info("setting up event handlers")
	greatdbInformer.Greatdb().V1alpha1().GreatDBBackupRecords().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.addenqueueFn,
		UpdateFunc: func(old, new interface{}) {
			newBcpRecord := new.(*gcv1alpha1.GreatDBBackupRecord)
			oldBackup := old.(*gcv1alpha1.GreatDBBackupRecord)
			// Skip processing if the versions are the same
			if newBcpRecord.ResourceVersion == oldBackup.ResourceVersion {
				return
			}
			controller.enqueueFn(new)
		},
		DeleteFunc: controller.enqueueFn,
	})

	// Add the event handling hook of job set
	dblog.Log.V(2).Info("Add the event handling hook of job set")
	kubeLabelInformer.Batch().V1().Jobs().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueJobFn,
		UpdateFunc: func(old, new interface{}) {
			newJob := new.(*batchv1.Job)
			oldJob := old.(*batchv1.Job)

			if newJob.ResourceVersion == oldJob.ResourceVersion {
				return
			}
			controller.enqueueJobFn(new)
		},
		DeleteFunc: controller.enqueueJobFn,
	})

	return controller

}

func (ctrl *GreatDbBackupRecordController) Run(threading int, stopCh <-chan struct{}) {
	// Capture crash
	defer utilruntime.HandleCrash()
	// close the queue
	defer ctrl.Queue.ShutDown()

	dblog.Log.Info("starting greatdb-backup-record controller")

	dblog.Log.V(2).Info("waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, ctrl.GreatdbBackupSynced); !ok {
		dblog.Log.Error("failed to wait for cache to sync")
		return
	}

	dblog.Log.V(2).Info("Starting workers")
	for i := 0; i < threading; i++ {
		go wait.Until(ctrl.runWorker, time.Second, stopCh)
	}

	<-stopCh
	dblog.Log.Info("shutting down greatdb-backup-record controller workers")

}

func (ctrl *GreatDbBackupRecordController) runWorker() {
	for ctrl.processNextWorkItem() {

	}
}

func (ctrl *GreatDbBackupRecordController) processNextWorkItem() bool {

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
			return fmt.Errorf("failed to  syncing %s : %s, requeuing", key, err.Error())

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
func (ctrl *GreatDbBackupRecordController) Sync(key string) error {
	dblog.Log.Infof("Start synchronizing greatdb backup record %s", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		dblog.Log.Errorf("invalid resource key %s", key)
		return nil
	}
	bcpRecord, err := ctrl.Listers.BackupRecordLister.GreatDBBackupRecords(ns).Get(name)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			dblog.Log.Errorf("Work queue does not have backup record %s:%s", ns, name)
			return nil
		}
	}

	newBcpRecord := bcpRecord.DeepCopy()

	// Set Default Configuration
	if ctrl.setDefault(newBcpRecord) {
		_, err = ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(bcpRecord.Namespace).Update(context.TODO(), newBcpRecord, metav1.UpdateOptions{})

		if err != nil {
			dblog.Log.Errorf("Failed to update backup %s default value, message: %s", bcpRecord.Name, err.Error())
			return err
		}
		dblog.Log.Info("Updating backup defaults succeeded")
	}

	// update backup
	if err = ctrl.syncBackupRecord(newBcpRecord); err != nil {
		dblog.Log.Reason(err).Errorf("Failed to update backup %s/%s", newBcpRecord.Namespace, newBcpRecord.Name)
		return err
	}

	//  update backup
	var updateBackup *gcv1alpha1.GreatDBBackupRecord
	if !reflect.DeepEqual(bcpRecord, newBcpRecord) {
		updateBackup, err = ctrl.updateGreatDBBackup(newBcpRecord)
		if err != nil {
			return err
		}
		newBcpRecord.ResourceVersion = updateBackup.ResourceVersion
	}

	if !reflect.DeepEqual(bcpRecord.Status, newBcpRecord.Status) {
		if err = ctrl.updateGreatDBBackupRecordStatus(newBcpRecord); err != nil {
			return err
		}
	}

	dblog.Log.Infof("Successfully synchronized backup %s/%s", newBcpRecord.Namespace, newBcpRecord.Name)

	return nil
}

func (ctrl GreatDbBackupRecordController) setDefault(bcpRecord *gcv1alpha1.GreatDBBackupRecord) bool {
	return true
}

// updateBackup Synchronize the backup state to the desired state
func (ctrl *GreatDbBackupRecordController) syncBackupRecord(bcpRecord *gcv1alpha1.GreatDBBackupRecord) (err error) {

	if !bcpRecord.DeletionTimestamp.IsZero() {

		err = ctrl.updateGreatDBBackupRecordStatus(bcpRecord)
		if err != nil {
			dblog.Log.Error(err.Error())
		}
	}

	// synchronize
	if err = ctrl.managers.Record.Sync(bcpRecord); err != nil {
		return err
	}

	if err = ctrl.startForegroundDeletion(bcpRecord); err != nil {
		return err
	}
	return nil
}

// updateGreatDBBackup
func (ctrl *GreatDbBackupRecordController) updateGreatDBBackup(backup *gcv1alpha1.GreatDBBackupRecord) (updateBackup *gcv1alpha1.GreatDBBackupRecord, err error) {

	oldBackup := backup.DeepCopy()
	// update fields
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

		updateBackup, err = ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(backup.Namespace).Update(context.TODO(), backup, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		upBackup, err1 := ctrl.Listers.BackupRecordLister.GreatDBBackupRecords(backup.Namespace).Get(backup.Name)
		if err1 != nil {
			dblog.Log.Errorf("error getting updated greatdb-backup %s/%s from lister,message: %s", backup.Namespace, backup.Name, err1.Error())
		} else {
			backup = upBackup.DeepCopy()
			// If there is no change, discard the update
			if reflect.DeepEqual(backup.Spec, oldBackup.Spec) {
				return nil
			}
			backup.Spec = oldBackup.Spec

		}
		dblog.Log.Errorf("failed to update backup  %s/%s, message: %s", backup.Namespace, backup.Name, err.Error())
		return err
	})

	if err != nil {
		dblog.Log.Errorf("failed to update backup %s/%s, message: %s", backup.Namespace, backup.Name, err.Error())
	}

	return

}

// UpdateGreatDBClusterStatus Update backup status
func (ctrl *GreatDbBackupRecordController) updateGreatDBBackupRecordStatus(backup *gcv1alpha1.GreatDBBackupRecord) error {
	oldBackup := backup.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(backup.Namespace).UpdateStatus(context.TODO(), backup, metav1.UpdateOptions{})

		if err == nil {
			return nil
		}
		upBackup, err1 := ctrl.Listers.BackupRecordLister.GreatDBBackupRecords(backup.Namespace).Get(backup.Name)
		if err1 != nil {
			dblog.Log.Errorf("error getting updated greatdb-backup %s/%s from lister,message: %s", backup.Namespace, backup.Name, err1.Error())
		} else {
			backup = upBackup.DeepCopy()
			// If there is no change, discard the update
			if reflect.DeepEqual(backup.Status, oldBackup.Status) {
				return nil
			}
			backup.Status = oldBackup.Status

		}

		dblog.Log.Errorf("failed to update backup %s/%s status, message: %s", backup.Namespace, backup.Name, err.Error())

		return err
	})

	if err != nil {
		dblog.Log.Errorf("failed to update backup %s/%s status, message: %s", backup.Namespace, backup.Name, err.Error())
	}

	return nil

}

func (ctrl GreatDbBackupRecordController) startForegroundDeletion(backup *gcv1alpha1.GreatDBBackupRecord) error {

	if backup.DeletionTimestamp.IsZero() {
		return nil
	}

	if len(backup.Finalizers) == 0 {
		return nil
	}
	patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
	_, err := ctrl.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(
		backup.Namespace).Patch(context.TODO(), backup.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})

	if err != nil {
		return err
	}
	return nil
}

// addenqueueFn When creating a backup, add the backup to the queue
func (ctrl *GreatDbBackupRecordController) addenqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		dblog.Log.Errorf("Invalid object:  %s", err.Error())
		return
	}
	dblog.Log.Infof("Listened to the greatdb-backup change event, changed object %s", key)

	ctrl.Queue.Add(key)

}

// enqueueFn When updating a backup, delay adding the backup to the queue
func (ctrl *GreatDbBackupRecordController) enqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		dblog.Log.Errorf("Invalid object:  %s", err.Error())
		return
	}
	dblog.Log.Infof("Listened to the greatdb-backup change event, changed object %s", key)
	// Wait five seconds before joining the queue
	ctrl.Queue.AddAfter(key, time.Duration(5*time.Second))
}

// enqueueJobFn When the satisfied stateful set changes, obtain the backup object from the stateful set and join the queue
func (ctrl *GreatDbBackupRecordController) enqueueJobFn(obj interface{}) {

	var key string
	var err error

	job := obj.(*batchv1.Job)

	ns := job.Namespace

	if value, ok := job.Labels[resources.AppKubeNameLabelKey]; !(ok && value == resources.AppKubeNameLabelValue) {
		return
	}

	backuprecordName, ok := job.Labels[resources.AppKubeBackupRecordNameLabelKey]
	if !ok {
		return
	}

	dblog.Log.Infof("Listened to the job change event, changed object %s", job.Name)

	_, err = ctrl.Listers.BackupRecordLister.GreatDBBackupRecords(ns).Get(backuprecordName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			dblog.Log.V(3).Infof("The greatdb backuprecordName instance %s/%s from job %s does not exist", ns, backuprecordName, job.Name)
			return
		}
		dblog.Log.Error(err.Error())
		return
	}

	key = fmt.Sprintf("%s/%s", ns, backuprecordName)

	// Wait five seconds before joining the queue
	ctrl.Queue.AddAfter(key, time.Duration(5*time.Second))
}
