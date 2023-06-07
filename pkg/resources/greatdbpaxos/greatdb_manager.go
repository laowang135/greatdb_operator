package greatdbpaxos

import (
	"context"
	"encoding/json"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/config"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	dblog "greatdb-operator/pkg/utils/log"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
)

type GreatDBManager struct {
	Client   *deps.ClientSet
	Lister   *deps.Listers
	Recorder record.EventRecorder
}

func (great *GreatDBManager) Sync(cluster *v1alpha1.GreatDBPaxos) (err error) {

	great.UpdateTargetInstanceToMember(cluster)

	if err = great.CreateOrUpdateGreatDB(cluster); err != nil {
		return err
	}

	if err = great.UpdateGreatDBStatus(cluster); err != nil {
		return err
	}

	return nil

}

func (great GreatDBManager) UpdateTargetInstanceToMember(cluster *v1alpha1.GreatDBPaxos) {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosDeployDB {
		return
	}

	cluster.Status.Instances = cluster.Spec.Instances
	cluster.Status.TargetInstances = cluster.Spec.Instances

	if cluster.Status.Member == nil {
		cluster.Status.Member = make([]v1alpha1.MemberCondition, 0)
	}

	num := len(cluster.Status.Member)

	if num >= int(cluster.Status.TargetInstances) {
		return
	}
	index := GetNextIndex(cluster.Status.Member)

	for num < int(cluster.Status.TargetInstances) {
		name := fmt.Sprintf("%s%s-%d", cluster.Name, resources.ComponentGreatDBSuffix, index)
		cluster.Status.Member = append(cluster.Status.Member, v1alpha1.MemberCondition{
			Name:       name,
			Index:      index,
			CreateType: v1alpha1.InitCreateMember,
			PvcName:    name,
		})
		num++
		index += 1

	}

}

func (great GreatDBManager) CreateOrUpdateGreatDB(cluster *v1alpha1.GreatDBPaxos) error {

	for _, member := range cluster.Status.Member {
		if err := great.CreateOrUpdateInstance(cluster, member); err != nil {
			return err
		}
	}

	return nil
}

func (great GreatDBManager) CreateOrUpdateInstance(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {

	ns := cluster.GetNamespace()

	if err := great.SyncPvc(cluster, member); err != nil {
		return err
	}

	if cluster.DeletionTimestamp.IsZero() {

		// pause
		pause, err := great.pauseGreatDB(cluster, member)
		if err != nil {
			return err
		}
		// If the instance is paused, end processing
		if pause {
			return nil
		}
	}

	del, err := great.deleteGreatDB(cluster, member)
	if err != nil {
		return err
	}

	if del {
		return nil
	}

	// create
	pod, err := great.Lister.PodLister.Pods(ns).Get(member.Name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// create greatdb instance
			if err = great.createGreatDBPod(cluster, member); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to sync greatDB pods of cluster %s/%s", ns, cluster.Name)
		return err
	}

	newPod := pod.DeepCopy()

	if cluster.DeletionTimestamp.IsZero() {
		// restart
		//if err := great.restartGreatDB(cluster, newPod); err != nil {
		//	return err
		//}

		if _, ok := cluster.Status.RestartMember.Restarting[pod.Name]; ok {
			return nil
		}

		// upgrade
		err = great.upgradeGreatDB(cluster, newPod)
		if err != nil {
			return err
		}

		if _, ok := cluster.Status.UpgradeMember.Upgrading[pod.Name]; ok {
			return nil
		}

	}

	// update meta
	if err = great.updateGreatDBPod(cluster, newPod); err != nil {
		return err
	}

	dblog.Log.Infof("Successfully update the greatdb pods %s/%s", ns, member.Name)
	return nil

}

func (great GreatDBManager) createGreatDBPod(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	pod := great.NewGreatDBPod(cluster, member)

	if pod == nil {
		return nil
	}

	_, err := great.Client.KubeClientset.CoreV1().Pods(cluster.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// The pods already exists, but for unknown reasons, the operator has not monitored the greatdb pod.
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(pod.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			_, err = great.Client.KubeClientset.CoreV1().Pods(cluster.Namespace).Patch(
				context.TODO(), pod.Name, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of pod, message: %s", err.Error())
				return err
			}
			return nil
		}

		dblog.Log.Errorf("failed to create  greatdb pod %s/%s  , message: %s", pod.Namespace, pod.Name, err.Error())
		return err
	}

	dblog.Log.Infof("Successfully created the greatdb pod %s/%s", pod.Namespace, pod.Name)

	return nil

}

func (great GreatDBManager) NewGreatDBPod(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (pod *corev1.Pod) {

	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	labels := resources.MegerLabels(cluster.Spec.Labels, great.GetLabels(cluster.Name))
	// TODO Debug
	labels[resources.AppKubePodLabelKey] = member.Name
	podSpec := great.GetPodSpec(cluster, member)
	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     cluster.Spec.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			Name:            member.Name,
			Namespace:       cluster.Namespace,
		},
		Spec: podSpec,
	}

	return
}

// GetLabels  Return to the default label settings
func (great GreatDBManager) GetLabels(clusterName string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = clusterName
	return

}

func (great GreatDBManager) GetPodSpec(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (podSpec corev1.PodSpec) {

	configmapName := cluster.Name + resources.ComponentGreatDBSuffix
	serviceName := cluster.Name + resources.ComponentGreatDBSuffix

	containers := great.newContainers(serviceName, cluster, member)
	volume := great.newVolumes(configmapName, member.PvcName)
	// update Affinity
	affinity := cluster.Spec.Affinity

	if affinity == nil {
		affinity = getDefaultAffinity(cluster.Name)
	}

	podSpec = corev1.PodSpec{

		Containers:        containers,
		Volumes:           volume,
		PriorityClassName: cluster.Spec.PriorityClassName,
		SecurityContext:   &cluster.Spec.PodSecurityContext,
		ImagePullSecrets:  cluster.Spec.ImagePullSecrets,
		Affinity:          affinity,
		Hostname:          member.Name,
		Subdomain:         serviceName,
	}

	return

}

func (great GreatDBManager) newContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (containers []corev1.Container) {

	db := great.newGreatDBContainers(serviceName, cluster)

	containers = append(containers, db)

	// Add another container

	return
}

func (great GreatDBManager) newGreatDBContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {

	env := great.newGreatDBEnv(serviceName, cluster)
	envForm := great.newGreatDBEnvForm(cluster.Spec.SecretName)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}

	container = corev1.Container{
		Name:            GreatDBContainerName,
		EnvFrom:         envForm,
		Env:             env,
		Command:         []string{"start-greatdb.sh"},
		Image:           cluster.Spec.Image,
		Resources:       cluster.Spec.Resources,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{ // config
				Name:      resources.GreatDBPvcConfigName,
				MountPath: greatdbConfigMountPath,
				ReadOnly:  true,
			},
			{ // data
				Name:      resources.GreatdbPvcDataName,
				MountPath: greatdbDataMountPath,
			},
		},
		ReadinessProbe: &corev1.Probe{
			PeriodSeconds:       5,
			InitialDelaySeconds: 30,
			FailureThreshold:    3,
			TimeoutSeconds:      2,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{Command: []string{"ReadinessProbe"}},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 60,
			PeriodSeconds:       10,
			FailureThreshold:    6,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{Command: []string{"ReadinessProbe"}},
			},
		},
	}

	if config.ServerVersion >= "v1.17.0" {
		container.StartupProbe = &corev1.Probe{
			PeriodSeconds:    30,
			FailureThreshold: 20,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(3306)},
			},
			TimeoutSeconds: 2,
		}
	}
	return
}

func (great GreatDBManager) newGreatDBEnv(serviceName string, cluster *v1alpha1.GreatDBPaxos) (env []corev1.EnvVar) {

	clusterDomain := cluster.Spec.ClusterDomain
	// user, pwd := resources.GetClusterUser(cluster)
	env = []corev1.EnvVar{
		{
			Name: "PODNAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
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
			Name:  "SERVICE_NAME",
			Value: serviceName,
		},
		{
			Name:  "CLUSTERDOMAIN",
			Value: clusterDomain,
		},
		{
			Name:  "FQDN",
			Value: "$(PODNAME).$(SERVICE_NAME).$(NAMESPACE).svc.$(CLUSTERDOMAIN)",
		},
		{
			Name:  "SERVERPORT",
			Value: fmt.Sprintf("%d", cluster.Spec.Port),
		}, //
		{
			Name:  "GROUPLOCALADDRESS",
			Value: "$(PODNAME).$(SERVICE_NAME).$(NAMESPACE).svc.$(CLUSTERDOMAIN)" + fmt.Sprintf(":%d", resources.GroupPort),
		},
		// {
		// 	Name:  resources.ClusterUserKey,
		// 	Value: user,
		// },
		// {
		// 	Name:  resources.ClusterUserPasswordKey,
		// 	Value: pwd,
		// },
	}

	return
}

func (great GreatDBManager) newGreatDBEnvForm(secretName string) (env []corev1.EnvFromSource) {
	env = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		},
	}
	return env
}

func (great GreatDBManager) newVolumes(configmapName, pvcName string) (volumes []corev1.Volume) {

	volumes = []corev1.Volume{}

	if configmapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: resources.GreatDBPvcConfigName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configmapName},
				},
			},
		})

	}

	if pvcName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: resources.GreatdbPvcDataName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}

	return

}

// updateGreatDBStatefulSet  Update greatdb statefulset
func (great GreatDBManager) updateGreatDBPod(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(podIns.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		_, err := great.Client.KubeClientset.CoreV1().Pods(podIns.Namespace).Patch(
			context.TODO(), podIns.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of pods  %s/%s,message: %s", podIns.Namespace, podIns.Name, err.Error())
		}

		return nil

	}
	needUpdate := false
	// update labels
	if up := great.updateLabels(podIns, cluster); up {
		needUpdate = true
	}

	// update annotations
	if up := great.updateAnnotations(podIns, cluster); up {
		needUpdate = true
	}

	// update Finalizers
	if podIns.ObjectMeta.Finalizers == nil {
		podIns.ObjectMeta.Finalizers = make([]string, 0, 1)
	}

	exist := false
	for _, value := range podIns.ObjectMeta.Finalizers {
		if value == resources.FinalizersGreatDBCluster {
			exist = true
			break
		}
	}
	if !exist {
		podIns.ObjectMeta.Finalizers = append(podIns.ObjectMeta.Finalizers, resources.FinalizersGreatDBCluster)
		needUpdate = true
	}

	// update OwnerReferences
	if podIns.ObjectMeta.OwnerReferences == nil {
		podIns.ObjectMeta.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist = false
	for _, value := range podIns.ObjectMeta.OwnerReferences {
		if value.UID == cluster.UID {
			exist = true
			break
		}
	}
	if !exist {
		owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
		podIns.ObjectMeta.OwnerReferences = []metav1.OwnerReference{owner}
		needUpdate = true
	}

	if needUpdate {

		err := great.updatePod(podIns)
		if err != nil {
			return err
		}

	}

	return nil

}

func (great GreatDBManager) updatePod(pod *corev1.Pod) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := great.Client.KubeClientset.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		upPod, err1 := great.Lister.PodLister.Pods(pod.Namespace).Get(pod.Name)
		if err1 != nil {
			dblog.Log.Reason(err).Errorf("failed to update pod %s/%s ", pod.Namespace, pod.Name)
		} else {
			if pod.ResourceVersion != upPod.ResourceVersion {
				pod.ResourceVersion = upPod.ResourceVersion
			}
		}

		return err
	})
	return err
}

func (great GreatDBManager) updateLabels(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	labels := resources.MegerLabels(cluster.Spec.Labels, great.GetLabels(cluster.Name))
	for key, value := range labels {
		if v, ok := pod.Labels[key]; !ok || v != value {
			pod.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (great GreatDBManager) updateAnnotations(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	for key, value := range cluster.Spec.Annotations {
		if v, ok := pod.Annotations[key]; !ok || v != value {
			pod.Annotations[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (great GreatDBManager) UpdateGreatDBStatus(cluster *v1alpha1.GreatDBPaxos) error {
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}
	var diag ClusterStatus
	if cluster.Status.Phase.Stage() > 3 {
		diag = great.probeStatusIfNeeded(cluster)
	} else {
		diag = great.ProbeStatus(cluster)
	}

	switch cluster.Status.Phase {

	case v1alpha1.GreatDBPaxosDeployDB:

		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosDeployDB, "")
		cluster.Status.Port = cluster.Spec.Port
		cluster.Status.Instances = cluster.Spec.Instances
		cluster.Status.TargetInstances = cluster.Spec.Instances
		cluster.Status.CurrentInstances = cluster.Spec.Instances

		if !diag.checkInstanceContainersIsReady() {
			return nil
		}
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosBootCluster, "")

	case v1alpha1.GreatDBPaxosBootCluster:

		if err := diag.createCluster(cluster); err != nil {
			return err
		}
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosInitUser, "")

	case v1alpha1.GreatDBPaxosInitUser:
		if err := diag.initUser(cluster); err != nil {
			return err
		}
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosSucceeded, "")

	case v1alpha1.GreatDBPaxosReady:
		// pause
		if cluster.Spec.Pause.Enable && cluster.Spec.Pause.Mode == v1alpha1.ClusterPause {
			msg := fmt.Sprintf("The cluster diagnosis status is %s", diag.status)
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosPause, msg)
			break
		}

		if cluster.Status.ReadyInstances < cluster.Spec.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosRepair, "")
		}

	case v1alpha1.GreatDBPaxosPause:

		if (cluster.Spec.Pause.Enable && cluster.Spec.Pause.Mode != v1alpha1.ClusterPause || !cluster.Spec.Pause.Enable) && cluster.Status.ReadyInstances > 0 {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	case v1alpha1.GreatDBPaxosRestart:

		if !cluster.Spec.Restart.Enable && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	case v1alpha1.GreatDBPaxosUpgrade:
		if len(cluster.Status.UpgradeMember.Upgrading) == 0 && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "upgrade successful")
		}
	case v1alpha1.GreatDBPaxosRepair:
		if cluster.Status.ReadyInstances < cluster.Spec.Instances {
			diag.repairCluster(cluster)
		}
	}

	return nil
}
