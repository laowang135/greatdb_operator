package greatdbbackup

import (
	"reflect"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	dblog "greatdb-operator/pkg/utils/log"
	"greatdb-operator/pkg/utils/tools"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
)

type GreatDBBackupScheduleManager struct {
	Client   *deps.ClientSet
	Listers  *deps.Listers
	Recorder record.EventRecorder
}

func (great *GreatDBBackupScheduleManager) Sync(backupSchedule *v1alpha1.GreatDBBackupSchedule) (err error) {

	cluster, err := great.Listers.PaxosLister.GreatDBPaxoses(backupSchedule.Namespace).Get(backupSchedule.Spec.ClusterName)
	if err != nil {

		if k8serrors.IsNotFound(err) {
			dblog.Log.Warningf("Cluster %s/%s does not exist", backupSchedule.Namespace, backupSchedule.Spec.ClusterName)
			return nil
		}
	}

	if cluster.Spec.Backup.Enable == nil || !*cluster.Spec.Backup.Enable {
		great.Recorder.Eventf(backupSchedule, corev1.EventTypeWarning, "cluster did not start backup service", "backup schedule terminated")
		return nil
	}

	if err = great.syncGreatDBBackUpScheduler(backupSchedule, cluster); err != nil {
		return err
	}

	return nil

}

// syncGreatDBBackUpScheduler  Synchronize backup schedule
func (great GreatDBBackupScheduleManager) syncGreatDBBackUpScheduler(backupSchedule *v1alpha1.GreatDBBackupSchedule, cluster *v1alpha1.GreatDBPaxos) error {
	cronMgr := deps.CronRegistryManager

	if backupSchedule.Status.Schedulers == nil {
		backupSchedule.Status.Schedulers = make([]v1alpha1.BackupScheduler, 0)
	}

	deleteScheduleList := make([]string, 0)

	if !backupSchedule.DeletionTimestamp.IsZero() {

		for _, scheduler := range backupSchedule.Status.Schedulers {
			if scheduler.Name == "" {
				continue
			}
			name := getBackupScheduleName(backupSchedule, scheduler)
			deleteScheduleList = append(deleteScheduleList, name)

		}
	} else if !backupSchedule.Spec.Suspend {

		nameMap := make(map[string]struct{})
		for _, bcp := range backupSchedule.Spec.Schedulers {
			if bcp.Name == "" {
				continue
			}

			if bcp.SelectStorage.Type == v1alpha1.BackupStorageNFS {
				if cluster.Spec.Backup.NFS == nil || cluster.Spec.Backup.NFS.Path == "" || cluster.Spec.Backup.NFS.Server == "" {
					great.Recorder.Eventf(backupSchedule, corev1.EventTypeWarning, "the cluster did not properly configure the backup nfs address", "%s schedule terminated", bcp.Name)
					continue
				}

			}
			if bcp.Clean != "" {
				_, err := tools.StringToDuration(bcp.Clean)
				if err != nil {
					dblog.Log.Errorf("Backup schedule %s/%s/%s field clean format error", backupSchedule.Namespace, backupSchedule.Name, bcp.Name)
					continue
				}
			}

			// If there is a backup plan with the same name, skip it directly
			name := getBackupScheduleName(backupSchedule, bcp)
			if _, ok := nameMap[name]; ok {
				continue
			}

			// Immediate backup
			if bcp.Schedule == "" {

				exist := false
				for _, statusBcp := range backupSchedule.Status.Schedulers {
					// Prevent immediate backups initiated multiple times due to updated schedules
					if bcp.Name == statusBcp.Name && statusBcp.Schedule == bcp.Schedule {
						exist = true
					}
				}
				if !exist {
					createBackupRecord(&cronMgr, backupSchedule, bcp)
				}

				continue
			}

			sch := BackupScheduleJob{}
			schRaw, ok := cronMgr.BackupJobs.Load(name)
			if ok {
				sch = schRaw.(BackupScheduleJob)
			}

			del := false
			if ok && (sch.Schedule != bcp.Schedule || sch.BackupType != bcp.BackupType ||
				sch.BackupResource != bcp.BackupResource || sch.Clean != bcp.Clean || !reflect.DeepEqual(sch.SelectStorage, bcp.SelectStorage)) {
				deleteScheduleList = append(deleteScheduleList, name)
				del = true
			}
			nameMap[name] = struct{}{}

			_, err := cron.ParseStandard(bcp.Schedule)
			if err != nil {
				continue
			}

			if !ok || del {
				jobID, err := cronMgr.AddFuncWithSeconds(bcp.Schedule, SyncBackupSchedule(&cronMgr, backupSchedule, bcp))
				if err != nil {
					dblog.Log.Reason(err).Errorf("can't parse cronjob schedule, backup name %s/%s, schedule %s", backupSchedule.Namespace, bcp.Name, bcp.Schedule)
					continue
				}

				deps.CronRegistryManager.BackupJobs.Store(bcp.Name, BackupScheduleJob{
					BackupScheduler: bcp,
					JobID:           jobID,
				})
			}

		}

		for i, scheduler := range backupSchedule.Status.Schedulers {
			if scheduler.Name == "" {
				continue
			}
			name := getBackupScheduleName(backupSchedule, scheduler)
			if !reflect.DeepEqual(scheduler, backupSchedule.Spec.Schedulers[i]) {
				deleteScheduleList = append(deleteScheduleList, name)
			}

		}

	}

	if backupSchedule.Spec.Suspend {
		for i, scheduler := range backupSchedule.Status.Schedulers {
			if scheduler.Name == "" {
				continue
			}
			name := getBackupScheduleName(backupSchedule, scheduler)
			if !reflect.DeepEqual(scheduler, backupSchedule.Spec.Schedulers[i]) {
				deleteScheduleList = append(deleteScheduleList, name)
			}

		}

		// Remove the modified backup plan
		for _, name := range deleteScheduleList {
			deleteBackupSchedule(&cronMgr, name)
		}
		backupSchedule.Status.Message = "all backup plans have been stopped"
		backupSchedule.Status.Schedulers = nil
		return nil

	}

	// Remove the modified backup plan
	for _, name := range deleteScheduleList {
		deleteBackupSchedule(&cronMgr, name)
	}

	backupSchedule.Status.Schedulers = backupSchedule.Spec.Schedulers

	return nil

}
