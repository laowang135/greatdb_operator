package greatdbbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/sidecar/backupServer"

	dblog "greatdb-operator/pkg/utils/log"

	"greatdb-operator/pkg/resources/internal"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
)

type GreatDBBackupRecordManager struct {
	Client   *deps.ClientSet
	Lister   *deps.Listers
	Recorder record.EventRecorder
}

func (great *GreatDBBackupRecordManager) Sync(backuprecord *v1alpha1.GreatDBBackupRecord) (err error) {

	cluster, err := great.Lister.PaxosLister.GreatDBPaxoses(backuprecord.Namespace).Get(backuprecord.Spec.ClusterName)
	if err != nil {
		return fmt.Errorf("cluster doesn't exist")
	}

	// If no instance is selected, search for available instances, priority: SECONDARY>PRIMARY
	if backuprecord.Spec.InstanceName == "" {
		hostInfo, err := great.GetBackupServerAddress(cluster)
		if err != nil {
			return err
		}
		backuprecord.Spec.InstanceName = great.SplitPod(hostInfo)
	}

	if backuprecord.Spec.BackupResource == v1alpha1.GreatDBBackupResourceType {
		return great.SyncGreatDB(backuprecord)
	}

	return nil

}

func (great *GreatDBBackupRecordManager) SyncGreatDB(backuprecord *v1alpha1.GreatDBBackupRecord) (err error) {

	clusterName := backuprecord.Spec.ClusterName

	cluster, err := great.Lister.PaxosLister.GreatDBPaxoses(backuprecord.Namespace).Get(clusterName)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionError
			backuprecord.Status.Message = fmt.Sprintf("not found cluster %s:%s", backuprecord.Namespace, clusterName)
		}
		dblog.Log.Reason(err).Errorf("failed to get GreatDBPaxos %s/%s", backuprecord.Namespace, clusterName)
		return err
	}

	if err = great.CreateOrUpdateGreatDBBackupRecord(backuprecord, cluster); err != nil {
		return err
	}

	if err = great.UpdateGreatDBBackupRecordStatus(backuprecord, cluster); err != nil {
		return err
	}

	return nil

}

func (great GreatDBBackupRecordManager) CreateOrUpdateGreatDBBackupRecord(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) error {
	// The backuprecord starts to clean, and no more resources need to be created
	if !backuprecord.DeletionTimestamp.IsZero() {
		return nil
	}

	if backuprecord.Status.Status == v1alpha1.GreatDBBackupRecordConditionSuccess ||
		backuprecord.Status.Status == v1alpha1.GreatDBBackupRecordConditionError {
		return nil
	}

	if !backuprecord.DeletionTimestamp.IsZero() {
		return nil
	}

	backupName, ok := backuprecord.Labels[resources.AppKubeBackupNameLabelKey]
	if ok || backupName != "" {

		backupScheduler, err := great.Lister.BackupSchedulerLister.GreatDBBackupSchedules(backuprecord.Namespace).Get(backupName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				dblog.Log.Infof("backupScheduler is not found, backup record name %v error", backuprecord.Name)
				backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionError
				backuprecord.Status.Message = fmt.Sprintf("The backup plan %s/%s has been deleted, and this backup will no longer be performed", backuprecord.Namespace, backupName)
				return nil
			}
			return err
		}

		if !backupScheduler.DeletionTimestamp.IsZero() {
			backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionError
			backuprecord.Status.Message = fmt.Sprintf("The backup plan %s/%s has been deleted, and this backup will no longer be performed", backuprecord.Namespace, backupName)
			return nil
		}

		ownerReferences := []metav1.OwnerReference{resources.GetGreatDBBackupRecordOwnerReferences(backupScheduler.Name, backupScheduler.UID)}

		if backuprecord.OwnerReferences == nil {
			backuprecord.OwnerReferences = ownerReferences
		}

	}

	if backuprecord.Spec.BackupType == v1alpha1.BackupTypeIncrement {
		if len(backuprecord.Status.BackupPath) == 0 {
			lastBackup, err := great.getLatestSuccessfulBackup(backuprecord)
			if err != nil {
				return err
			}
			if lastBackup == nil {
				// backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionError
				backuprecord.Status.Message = "No full backup was found, incremental backup relies on full backup, so full backup will be performed this time"
				backuprecord.Spec.BackupType = v1alpha1.BackupTypeFull
				return nil
			}
			backuprecord.Status.BackupPath = append(lastBackup.Status.BackupPath, backuprecord.Name)
			backuprecord.Status.BackupIncFromLsn = lastBackup.Status.ToLsn
		}
	} else {
		backuprecord.Status.BackupPath = []string{backuprecord.Name}
	}

	ns := backuprecord.GetNamespace()

	_, err := great.Lister.JobLister.Jobs(ns).Get(backuprecord.Name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// create greatdb sts
			if err = great.createGreatDBackupRecordJob(backuprecord, cluster); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Reason(err).Errorf("Failed to obtain backup job for backup record  %s/%s", ns, backuprecord.Name)
		return err
	}

	dblog.Log.Infof("Greatdb backup record %s/%s updated successfully ", ns, backuprecord.Name)

	return nil
}

func (great GreatDBBackupRecordManager) createGreatDBackupRecordJob(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) error {

	job, err := great.NewGreatDBBackupRecordJob(backuprecord, cluster)
	if err != nil {
		return err
	}

	_, err = great.Client.KubeClientset.BatchV1().Jobs(backuprecord.Namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}

		dblog.Log.Reason(err).Errorf("failed to create  backup job %s/%s ", job.Namespace, job.Name)
		return err
	}

	dblog.Log.Infof("Successfully created the greatdb backup job %s/%s", job.Namespace, job.Name)

	return nil

}

func (great GreatDBBackupRecordManager) NewGreatDBBackupRecordJob(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (job *batchv1.Job, err error) {

	jobSpec, err := great.GetGreatDBJobSpec(backuprecord, cluster)
	if err != nil {
		return nil, err
	}

	owner := resources.GetGreatDBBackupRecordOwnerReferences(backuprecord.Name, backuprecord.UID)

	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            backuprecord.Name,
			Namespace:       backuprecord.Namespace,
			Labels:          great.getLabels(backuprecord.Spec.ClusterName, backuprecord.Name),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: jobSpec,
	}

	return
}

func (great GreatDBBackupRecordManager) GetGreatDBJobSpec(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (jobSpec batchv1.JobSpec, err error) {

	ns := backuprecord.Namespace

	InstanceName := backuprecord.Spec.InstanceName
	_, err = great.Lister.PodLister.Pods(ns).Get(InstanceName)
	if err != nil {
		dblog.Log.Reason(err).Error("Failed to obtain backup target pod")
		return
	}

	containers := great.newGreatDBContainers(backuprecord, cluster)

	backoffLimit := int32(1)

	jobSpec = batchv1.JobSpec{
		BackoffLimit: &backoffLimit,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: great.getLabels(backuprecord.Spec.ClusterName, backuprecord.Name),
			},
			Spec: corev1.PodSpec{
				RestartPolicy:    corev1.RestartPolicyNever,
				Containers:       containers,
				SecurityContext:  &cluster.Spec.PodSecurityContext,
				ImagePullSecrets: cluster.Spec.ImagePullSecrets,
			},
		},
	}

	return

}

func (great GreatDBBackupRecordManager) newGreatDBContainers(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (containers []corev1.Container) {

	db := great.newGreatDBBackupContainers(backuprecord, cluster)

	containers = append(containers, db)

	return
}

func (great GreatDBBackupRecordManager) newGreatDBBackupContainers(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {

	env := great.newGreatDBBackupRecordEnv(backuprecord, cluster)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}
	resource := corev1.ResourceRequirements{}
	resource.Requests = make(corev1.ResourceList)
	resource.Limits = make(corev1.ResourceList)

	resource.Requests[corev1.ResourceCPU] = k8sresource.MustParse("100m")
	resource.Limits[corev1.ResourceCPU] = cluster.Spec.Resources.Requests[corev1.ResourceCPU]
	resource.Requests[corev1.ResourceMemory] = k8sresource.MustParse("50Mi")
	resource.Limits[corev1.ResourceMemory] = cluster.Spec.Resources.Requests[corev1.ResourceMemory]

	container = corev1.Container{
		Name:            "greatdb-backup",
		Env:             env,
		Command:         []string{"greatdb-agent", "--mode=greatdb-backup"},
		Image:           cluster.Spec.Image,
		Resources:       resource,
		ImagePullPolicy: imagePullPolicy,
	}

	return
}

func (great GreatDBBackupRecordManager) newGreatDBBackupRecordEnv(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) (env []corev1.EnvVar) {
	svcName := cluster.Name + resources.ComponentGreatDBSuffix
	backupServerAddress := fmt.Sprintf("%s.%s.%s.svc.%s", backuprecord.Spec.InstanceName, svcName, backuprecord.Namespace, cluster.Spec.ClusterDomain)
	// TODO DEBUG
	// backupServerAddress := resources.GetInstanceFQDN(backuprecord.Spec.ClusterName, backuprecord.Spec.InstanceName, backuprecord.Namespace, cluster.Spec.ClusterDomain)
	env = []corev1.EnvVar{
		{
			Name: "NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		},
		{
			Name:  "BackupDestination",
			Value: backuprecord.Name,
		},
		{
			Name:  "BackupStorage",
			Value: string(backuprecord.Spec.SelectStorage.Type),
		},
		{
			Name:  "BackupType",
			Value: string(backuprecord.Spec.BackupType),
		},
		{
			Name:  "BackupServerAddress",
			Value: backupServerAddress,
		},
		{
			Name:  "BackupServerPort",
			Value: strconv.Itoa(resources.BackupServerPort),
		},
		{
			Name:  "BackupResource",
			Value: string(backuprecord.Spec.BackupResource),
		},
	}

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

	// if backuprecord.Spec.SelectStorage.Type == v1alpha1.BackupStorageUploadServer {
	// 	uploadServerEnv := []corev1.EnvVar{
	// 		{
	// 			Name:  "UploadServerAddress",
	// 			Value: backuprecord.Spec.SelectStorage.UploadServer.Address,
	// 		},
	// 		{
	// 			Name:  "UploadServerPort",
	// 			Value: strconv.Itoa(backuprecord.Spec.SelectStorage.UploadServer.Port),
	// 		},
	// 	}
	// 	env = append(env, uploadServerEnv...)
	// }

	if backuprecord.Spec.BackupType == v1alpha1.BackupTypeIncrement {

		incEnv := []corev1.EnvVar{
			{
				Name:  "BackupIncFromLsn",
				Value: backuprecord.Status.BackupIncFromLsn,
			},
		}
		env = append(env, incEnv...)

	}

	return
}

func (great GreatDBBackupRecordManager) UpdateGreatDBBackupRecordStatus(backuprecord *v1alpha1.GreatDBBackupRecord, cluster *v1alpha1.GreatDBPaxos) error {

	if backuprecord.Status.Status == v1alpha1.GreatDBBackupRecordConditionError || backuprecord.Status.Status == v1alpha1.GreatDBBackupRecordConditionSuccess {
		return nil
	}

	namespace := backuprecord.GetNamespace()

	job, err := great.Lister.JobLister.Jobs(namespace).Get(backuprecord.Name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			dblog.Log.Infof("The greatdb job %s/%s of backuprecord %s does not exist", namespace, backuprecord.Name, backuprecord.Name)
			return nil
		}
		dblog.Log.Error(err.Error())
		return err
	}

	switch {
	case job.Status.Active == 1:
		backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionRunning
	case job.Status.Succeeded == 1:

		err := great.getBackupInfo(backuprecord, cluster.Spec.ClusterDomain)
		if err != nil {
			dblog.Log.Error(err.Error())
			return err
		}
		backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionSuccess
		backuprecord.Status.CompletedAt = job.Status.CompletionTime

	case job.Status.Failed > 1:
		backuprecord.Status.Status = v1alpha1.GreatDBBackupRecordConditionError
	}

	return nil
}

func (great GreatDBBackupRecordManager) getLatestSuccessfulBackup(backuprecord *v1alpha1.GreatDBBackupRecord) (*v1alpha1.GreatDBBackupRecord, error) {

	selectLabel, _ := labels.Parse(
		fmt.Sprintf(
			"%s=%s,%s=%s",
			resources.AppKubeInstanceLabelKey,
			backuprecord.Spec.ClusterName,
			resources.AppKubeBackupResourceTypeLabelKey,
			backuprecord.Spec.BackupResource,
		),
	)

	records, err := great.Lister.BackupRecordLister.GreatDBBackupRecords(backuprecord.Namespace).List(selectLabel)
	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to list backup record")
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}
	var latest *v1alpha1.GreatDBBackupRecord
	for _, record := range records {
		if record.Status.Status != v1alpha1.GreatDBBackupRecordConditionSuccess || len(record.Status.BackupPath) == 0 {
			continue
		}
		if record.Status.ToLsn == "" {
			continue
		}

		if latest == nil {
			latest = record
			continue
		}

		if latest.Status.CompletedAt.Before(record.Status.CompletedAt) {
			latest = record
		}
	}

	return latest, nil

}

// getLabels  Return to the default label settings
func (great GreatDBBackupRecordManager) getLabels(clusterName, name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = clusterName
	labels[resources.AppKubeBackupRecordNameLabelKey] = name
	return

}

func (great GreatDBBackupRecordManager) getBackupInfo(backuprecord *v1alpha1.GreatDBBackupRecord, clusterDomain string) error {
	backupReq := backupServer.BackupInfoRequest{
		BackupResource: string(backuprecord.Spec.BackupResource),
		Name:           backuprecord.Name,
	}
	data, err := json.Marshal(backupReq)
	if err != nil {
		dblog.Log.Errorf("%v, failed to unmarshal backup config", err)
		return fmt.Errorf("backup failed")
	}

	// TODO DEBUG
	// backupServerAddress := resources.GetInstanceFQDN(backuprecord.Spec.ClusterName, backuprecord.Spec.InstanceName, backuprecord.Namespace, clusterDomain)
	// server := fmt.Sprintf("http://%s:%d%s", backupServerAddress, backupServer.ServerPort, backupServer.ServerBackupInfoEndpoint)

	server := fmt.Sprintf("http://%s:%d%s", "172.17.120.142", 30031, backupServer.ServerBackupInfoEndpoint)

	resp, err := http.Post(server,
		"application/json",
		bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("backup failed, http error")
	}
	backupRep := backupServer.BackupInfoResponse{}
	if err := json.Unmarshal(body, &backupRep); err != nil {
		dblog.Log.Errorf("%v, failed to unmarshal backup response", err)
		return err
	}
	backuprecord.Status.BackupType = backupRep.BackupType
	backuprecord.Status.FromLsn = backupRep.FromLsn
	backuprecord.Status.ToLsn = backupRep.ToLsn
	backuprecord.Status.LastLsn = backupRep.LastLsn
	return nil
}

func (great GreatDBBackupRecordManager) GetBackupServerAddress(cluster *v1alpha1.GreatDBPaxos) (string, error) {

	memberList := great.getDataServerList(cluster)

	instanceName := make([]string, 0)
	primaryIns := ""

	for _, member := range memberList {

		if v1alpha1.MemberConditionType(member.State).Parse() != v1alpha1.MemberStatusOnline {
			continue
		}
		if v1alpha1.MemberRoleType(member.Role).Parse() == v1alpha1.MemberRolePrimary {
			primaryIns = member.Host
		} else {
			instanceName = append(instanceName, member.Host)
		}

	}

	if len(instanceName) > 0 {
		if len(instanceName) == 1 {
			return instanceName[0], nil
		}
		rand.Seed(time.Now().UnixNano())
		return instanceName[rand.Intn(len(instanceName))], nil
	}

	if primaryIns != "" {
		return primaryIns, nil
	}
	dblog.Log.Errorf("Cluster %s/%s has no members available for backup", cluster.Namespace, cluster.Name)
	return "", fmt.Errorf("cluster %s/%s has no members available for backup", cluster.Namespace, cluster.Name)
}

func (great GreatDBBackupRecordManager) getDataServerList(cluster *v1alpha1.GreatDBPaxos) []resources.PaxosMember {

	ns := cluster.Namespace
	clusterName := cluster.Name
	clusterDomain := cluster.Spec.ClusterDomain
	port := int(cluster.Spec.Port)
	user, pwd := resources.GetClusterUser(cluster)
	sqlClient := internal.NewDBClient()
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}
		host := resources.GetInstanceFQDN(clusterName, member.Name, ns, clusterDomain)
		err := sqlClient.Connect(user, pwd, host, port, "mysql")
		if err != nil {
			continue
		}
		memberList := make([]resources.PaxosMember, 0)

		err = sqlClient.Query(resources.QueryClusterMemberStatus, &memberList, resources.QueryClusterMemberFields)
		if err != nil {
			sqlClient.Close()
			dblog.Log.Reason(err).Errorf("failed to query cluster status")
			continue
		}
		sqlClient.Close()
		for _, status := range memberList {
			if status.State == string(v1alpha1.MemberStatusOnline) {
				return memberList
			}
		}
	}
	return []resources.PaxosMember{}
}

func (great GreatDBBackupRecordManager) SplitHost(hostInfo string) (string, string) {
	s := strings.Split(hostInfo, ":")
	if len(s) > 1 {
		return s[0], s[1]
	}

	return s[0], ""
}

func (great GreatDBBackupRecordManager) SplitPod(hostInfo string) string {
	host, _ := great.SplitHost(hostInfo)
	s := strings.Split(host, ".")
	return s[0]
}
