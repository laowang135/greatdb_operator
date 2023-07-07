package greatdbbackup

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	dblog "greatdb-operator/pkg/utils/log"
	"greatdb-operator/pkg/utils/tools"
)

func SyncBackupSchedule(r *deps.CronRegistry, greatdbbackup *v1alpha1.GreatDBBackupSchedule, scheduler v1alpha1.BackupScheduler) func() {
	return func() {
		createBackupRecord(r, greatdbbackup, scheduler)
		cleanBackupRecord(r, greatdbbackup, scheduler)

	}
}

// cleanBackupRecord Clean up the backups corresponding to the incoming backup plan
func cleanBackupRecord(r *deps.CronRegistry, backupSch *v1alpha1.GreatDBBackupSchedule, scheduler v1alpha1.BackupScheduler) {

	if scheduler.Clean == "" {
		return
	}

	scheduleName := backupSch.Name + "-" + scheduler.Name

	continueQuery := ""
	first := true
	duration, err := tools.StringToDuration(scheduler.Clean)
	if err != nil {
		return
	}
	now := resources.GetNowTime().Add(duration * -1)
	for first || continueQuery != "" {
		backupList, err := r.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(backupSch.Namespace).List(context.TODO(),
			metav1.ListOptions{
				LabelSelector: fmt.Sprintf(`%s=%s,%s=%s`, resources.AppKubeInstanceLabelKey, backupSch.Spec.ClusterName, resources.AppKubeBackupScheduleNameLabelKey, scheduleName),
				Limit:         100,
				Continue:      continueQuery,
			},
		)

		if err != nil {
			dblog.Log.Reason(err).Errorf("failed to list GreatDBBackupRecords")
			return
		}

		continueQuery = backupList.Continue
		for _, backupRecord := range backupList.Items {

			if !backupRecord.DeletionTimestamp.IsZero() {
				continue
			}

			if backupRecord.Status.Status == v1alpha1.GreatDBBackupRecordConditionError {
				if !backupRecord.CreationTimestamp.Time.Before(now) {
					continue
				}
			}

			// Skip if the backup completion time is not before the set cleaning time
			if backupRecord.Status.CompletedAt == nil || !backupRecord.Status.CompletedAt.Time.Before(now) {
				continue
			}

			// Delete the record, and submit the specific content to the record controller for processing
			err = r.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(backupRecord.Namespace).Delete(context.TODO(), backupRecord.Name, metav1.DeleteOptions{})

			if err != nil {
				if k8serrors.IsNotFound(err) {
					return
				}
				dblog.Log.Reason(err).Errorf("failed to delete %s/%s", backupRecord.Namespace, backupRecord.Name)
			}

		}

	}

}

func createBackupRecord(r *deps.CronRegistry, greatdbbackup *v1alpha1.GreatDBBackupSchedule, scheduler v1alpha1.BackupScheduler) error {

	now := resources.GetNowTime().Format("20060102150405")
	recordName := fmt.Sprintf("%s-%s-%s-%s", scheduler.BackupResource, scheduler.BackupType, greatdbbackup.Name, now)
	sch, err := r.Client.Clientset.GreatdbV1alpha1().GreatDBBackupSchedules(greatdbbackup.Namespace).Get(context.TODO(), greatdbbackup.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			name := getBackupScheduleName(greatdbbackup, scheduler)
			deleteBackupSchedule(r, name)
			return nil
		}
		dblog.Log.Reason(err).Errorf("failed to get GreatDBBackupSchedules %s/%s ", greatdbbackup.Namespace, greatdbbackup.Name)
		return err

	}

	owner := resources.GetGreatDBBackupSchedulerOwnerReferences(greatdbbackup.Name, greatdbbackup.UID)

	backupRcord := &v1alpha1.GreatDBBackupRecord{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       greatdbbackup.Namespace,
			Name:            recordName,
			Labels:          getLabels(greatdbbackup.Spec.ClusterName, greatdbbackup.Name, scheduler),
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: v1alpha1.GreatDBBackupRecordSpec{
			ClusterName:    greatdbbackup.Spec.ClusterName,
			InstanceName:   sch.Spec.InstanceName,
			BackupType:     scheduler.BackupType,
			BackupResource: scheduler.BackupResource,
			SelectStorage:  scheduler.SelectStorage,
			Clean:          scheduler.Clean,
		},
	}

	_, err = r.Client.Clientset.GreatdbV1alpha1().GreatDBBackupRecords(greatdbbackup.Namespace).Create(context.TODO(), backupRcord, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		dblog.Log.Reason(err).Errorf("failed to create backup")
		return err
	}
	dblog.Log.Infof("success to create backup record: %s/%s", greatdbbackup.Namespace, recordName)

	return nil
}

func deleteBackupSchedule(r *deps.CronRegistry, name string) {
	job, ok := r.BackupJobs.LoadAndDelete(name)
	if !ok {
		return
	}
	r.Crons.Remove(job.(BackupScheduleJob).JobID)
}

type BackupScheduleJob struct {
	v1alpha1.BackupScheduler
	JobID cron.EntryID
}

func getBackupScheduleName(backup *v1alpha1.GreatDBBackupSchedule, scheduler v1alpha1.BackupScheduler) string {

	return fmt.Sprintf("%s/%s/%s", backup.Namespace, backup.Name, scheduler.Name)

}

// getLabels  Return to the default label settings
func getLabels(clusterName, backName string, schedule v1alpha1.BackupScheduler) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = clusterName
	labels[resources.AppKubeBackupScheduleNameLabelKey] = backName + "-" + schedule.Name
	labels[resources.AppKubeBackupNameLabelKey] = backName
	// AppKubeBackupResourceTypeLabelKey
	labels[resources.AppKubeBackupResourceTypeLabelKey] = string(schedule.BackupResource)
	return

}
