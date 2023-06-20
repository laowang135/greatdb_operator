package greatdbpaxos

import (
	"fmt"
	"strconv"
	"strings"

	v1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
)

func (great GreatDBManager) newGreatDBBackupContainers(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {

	env := great.newGreatDBBackupRestoreEnv(backuprecord, cluster)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}

	volumeMounts := []corev1.VolumeMount{
		{ // backup
			Name:      "backup-pvc",
			MountPath: greatdbDataMountPath,
		},
	}

	resource := corev1.ResourceRequirements{}
	resource.Requests = make(corev1.ResourceList)
	resource.Limits = make(corev1.ResourceList)

	resource.Requests[corev1.ResourceCPU] = k8sresource.MustParse("2Gi")
	resource.Limits[corev1.ResourceCPU] = cluster.Spec.Resources.Requests[corev1.ResourceCPU]
	resource.Requests[corev1.ResourceMemory] = k8sresource.MustParse("2Gi")
	resource.Limits[corev1.ResourceMemory] = cluster.Spec.Resources.Requests[corev1.ResourceMemory]

	container = corev1.Container{
		Name:            "greatdb-restore-job",
		Env:             env,
		Command:         []string{"greatdb-agent", "--mode=greatdb-restore"},
		Image:           cluster.Spec.Image,
		Resources:       resource,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts:    volumeMounts,
	}

	return
}

func (great GreatDBManager) newGreatDBBackupRestoreEnv(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (env []corev1.EnvVar) {

	// TODO DEBUG
	// backupServerAddress := resources.GetInstanceFQDN(backuprecord.Spec.ClusterName, backuprecord.Spec.InstanceName, backuprecord.Namespace, cluster.Spec.ClusterDomain)

	svcName := cluster.Name + resources.ComponentGreatDBSuffix
	backupServerAddress := fmt.Sprintf("%s.%s.%s.svc.%s", backuprecord.Spec.InstanceName, svcName, backuprecord.Namespace, cluster.Spec.ClusterDomain)
	env = []corev1.EnvVar{
		{
			Name:  "BackupServerAddress",
			Value: backupServerAddress,
		},
		{
			Name:  "BackupServerPort",
			Value: strconv.Itoa(resources.BackupServerPort),
		},
		{
			Name:  "BackupRecordFiles",
			Value: strings.Join(backuprecord.Status.BackupPath, ","),
		},
	}

	storageEnv := []corev1.EnvVar{
		{
			Name:  "BackupStorage",
			Value: string(backuprecord.Spec.SelectStorage.Type),
		},
	}

	env = append(env, storageEnv...)

	if backuprecord.Spec.SelectStorage.Type == v1alpha1.BackupStorageS3 && backuprecord.Spec.SelectStorage.S3 != nil {
		s3Env := []corev1.EnvVar{
			{
				Name:  "BackupS3Bucket",
				Value: backuprecord.Spec.SelectStorage.S3.Bucket,
			},
			{
				Name:  "BackupS3EndpointURL",
				Value: backuprecord.Spec.SelectStorage.S3.EndpointURL,
			},
			{
				Name:  "BackupS3AccessKey",
				Value: backuprecord.Spec.SelectStorage.S3.AccessKey,
			},
			{
				Name:  "BackupS3SecretKey",
				Value: backuprecord.Spec.SelectStorage.S3.SecretKey,
			},
		}
		env = append(env, s3Env...)

	}

	if backuprecord.Spec.SelectStorage.Type == v1alpha1.BackupStorageUploadServer && backuprecord.Spec.SelectStorage.UploadServer != nil {
		uploadServerEnv := []corev1.EnvVar{
			{
				Name:  "UploadServerAddress",
				Value: backuprecord.Spec.SelectStorage.UploadServer.Address,
			},
			{
				Name:  "UploadServerPort",
				Value: strconv.Itoa(resources.BackupServerPort),
			},
		}
		env = append(env, uploadServerEnv...)
	}

	return
}

func (great GreatDBManager) getLatestSuccessfulBackup(ns, clustername, restoreType string) *v1alpha1.GreatDBBackupRecord {

	selectLabel, _ := labels.Parse(
		fmt.Sprintf(
			"%s=%s,%s=%s",
			resources.AppKubeInstanceLabelKey,
			clustername,
			resources.AppKubeBackupResourceTypeLabelKey,
			restoreType,
		),
	)
	records, err := great.Lister.BackupRecordLister.GreatDBBackupRecords(ns).List(selectLabel)
	if err != nil {
		return nil
	}

	if len(records) == 0 {
		return nil
	}
	var latest *v1alpha1.GreatDBBackupRecord
	for _, record := range records {
		if record.Status.Status != v1alpha1.GreatDBBackupRecordConditionSuccess || len(record.Status.BackupPath) == 0 {
			continue
		}
		if record.Spec.BackupResource == v1alpha1.GreatDBBackupResourceType && record.Status.ToLsn == "" {
			continue
		}

		if latest == nil {
			latest = record
			continue
		}

		if latest.ObjectMeta.CreationTimestamp.Before(&record.ObjectMeta.CreationTimestamp) {
			latest = record
		}
	}

	return latest

}
