package greatdbpaxos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/resources/service"
	dblog "greatdb-operator/pkg/utils/log"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	
)

type DashboardManager struct {
	Client   *deps.ClientSet
	Lister   *deps.Listers

}

func (dashboard *DashboardManager) Sync(cluster *v1alpha1.GreatDBPaxos) (err error) {

	if !cluster.Spec.Dashboard.Enable {
		return nil
	}

	if cluster.Status.Status != v1alpha1.ClusterStatusOnline{
		return nil
	}

	

	if err = dashboard.CreateOrUpdateDashboard(cluster); err != nil {

		return nil
	}
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	if err = dashboard.SyncClusterTopo(cluster); err == nil {
		return nil
	}

	return nil

}

func (dashboard DashboardManager) SyncClusterTopo(cluster *v1alpha1.GreatDBPaxos) error {

	if metav1.Now().Sub(cluster.Status.Dashboard.LastSyncTime.Time) < time.Minute*1 {
		return nil
	}

	ns := cluster.Namespace
	clusterDomain := cluster.GetClusterDomain()
	serverAddr := cluster.Name + resources.ComponentGreatDBSuffix
	// Get the cluster account password from secret
	user, password := resources.GetClusterUser(cluster)

	syncCluster := make(map[string]interface{})
	syncCluster["address"] = serverAddr
	syncCluster["port"] = fmt.Sprintf("%d", cluster.Spec.Port)
	syncCluster["cluster_name"] = cluster.Name
	syncCluster["username"] = user
	syncCluster["password"] = password
	bytesData, err := json.Marshal(syncCluster)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	reader := bytes.NewReader(bytesData)

	dashboardName := cluster.Name + resources.ComponentDashboardSuffix
	svcName := dashboardName
	syncUrlFmt := "http://%s.%s.%s.svc.%s:8080/gdbc/api/v1/cluster/init_cluster/"
	dashboardSyncUrl := fmt.Sprintf(syncUrlFmt, dashboardName, svcName, ns, clusterDomain)

	// dashboardSyncUrl = "http://172.17.120.143:30500/gdbc/api/v1/cluster/init_cluster/"

	request, err := http.NewRequest("POST", dashboardSyncUrl, reader)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	syncRes := fmt.Sprintf("Sync cluster response StatusCode %d:", resp.StatusCode)
	if resp.StatusCode == 200 {
		cluster.Status.Dashboard.Ready = true
		cluster.Status.Dashboard.LastSyncTime = metav1.Now()
	} else {
		cluster.Status.Dashboard.Ready = false
	}
	dblog.Log.Info(syncRes)

	defer resp.Body.Close()

	return nil
}

func (dashboard DashboardManager) CreateOrUpdateDashboard(cluster *v1alpha1.GreatDBPaxos) error {

	dashBoardName := cluster.Name + resources.ComponentDashboardSuffix
	ns := cluster.GetNamespace()

	pod, err := dashboard.Lister.PodLister.Pods(ns).Get(dashBoardName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// create dashboard pods
			if err = dashboard.createDashboard(cluster); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to sync greatDB statefulset of cluster %s/%s", ns, cluster.Name)
		return err
	}
	newPod := pod.DeepCopy()
	if err = dashboard.updateDashboard(cluster, newPod); err != nil {
		return err
	}

	dblog.Log.Infof("Successfully update the greatdb statefulset %s/%s", ns, dashBoardName)

	return nil
}

func (dashboard DashboardManager) createDashboard(cluster *v1alpha1.GreatDBPaxos) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}
	pod := dashboard.newDashboardPod(cluster)

	if pod == nil {
		return nil
	}

	_, err := dashboard.Client.KubeClientset.CoreV1().Pods(cluster.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			labels, _ := json.Marshal(pod.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			_, err = dashboard.Client.KubeClientset.CoreV1().Pods(cluster.Namespace).Patch(
				context.TODO(), pod.Name, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of statefulset, message: %s", err.Error())
				return err
			}
			return nil
		}

		dblog.Log.Errorf("failed to create  the dashboard  pods %s/%s  , message: %s", pod.Namespace, pod.Name, err.Error())
		return err
	}

	dblog.Log.Infof("Successfully created the dashboard  pods %s/%s", pod.Namespace, pod.Name)

	return nil

}



// GetLabels  Return to the default label settings
func (dashboard DashboardManager) GetLabels(name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = name
	labels[resources.AppKubeComponentLabelKey] = resources.AppKubeComponentDashboard


	return

}

func (dashboard DashboardManager) newDashboardPod(cluster *v1alpha1.GreatDBPaxos) (pod *corev1.Pod) {
	name := cluster.Name + resources.ComponentDashboardSuffix
	secretName := cluster.Spec.SecretName

	
	serviceName := cluster.Name + resources.ComponentDashboardSuffix

	initContainers := dashboard.newInitContainers(serviceName, cluster)
	containers := dashboard.newContainers(secretName, serviceName, cluster)
	volume := dashboard.newVolumes( cluster)
	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	// update Affinity
	affinity := cluster.Spec.Affinity
	if cluster.Spec.Affinity != nil {
		affinity = cluster.Spec.Affinity
	}
	if affinity == nil {
		affinity = getDefaultAffinity(cluster.Name)
	}

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace: cluster.Namespace,
			Finalizers: []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
			Labels: dashboard.GetLabels(cluster.Name),
		},
		Spec: corev1.PodSpec{
			InitContainers:    initContainers,
			Containers:        containers,
			Volumes:           volume,
			PriorityClassName: cluster.Spec.PriorityClassName,
			SecurityContext:   &cluster.Spec.PodSecurityContext,
			ImagePullSecrets:  cluster.Spec.ImagePullSecrets,
			Affinity:          affinity,
		},
	}

	return 

}

func (dashboard DashboardManager) getVolumeClaimTemplates(cluster *v1alpha1.GreatDBPaxos) (pvcs []corev1.PersistentVolumeClaim) {

	if !cluster.Spec.Dashboard.PersistentVolumeClaimSpec.Resources.Requests.Storage().IsZero() {
		pvcs = append(pvcs, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: resources.GreatdbPvcDataName,
			},
			Spec: cluster.Spec.Dashboard.PersistentVolumeClaimSpec,
		})
	}
	return

}

func (dashboard DashboardManager) newInitContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos) (containers []corev1.Container) {

	init := dashboard.newInitDashboardContainers(serviceName, cluster)

	containers = append(containers, init)

	return
}

func (dashboard DashboardManager) newContainers(secretName, serviceName string, cluster *v1alpha1.GreatDBPaxos) (containers []corev1.Container) {

	db := dashboard.newDashboardContainers(secretName, serviceName, cluster)
	containers = append(containers, db)
	return
}

func (dashboard DashboardManager) newDashboardContainers(secretName, serviceName string, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {
	clusterDomain := cluster.GetClusterDomain()

	user, pwd := resources.GetClusterUser(cluster)
	env := dashboard.newDashboardEnv(serviceName, clusterDomain, user, pwd, cluster)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}

	container = corev1.Container{
		Name:            DashboardContainerName,
		Env:             env,
		Image:           cluster.Spec.Dashboard.Image,
		Resources:       cluster.Spec.Dashboard.Resources,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{ // data
				Name:      resources.GreatdbPvcDataName,
				MountPath: dashboardDataMountPath,
			},
		},
		ReadinessProbe: &corev1.Probe{
			PeriodSeconds:       10,
			InitialDelaySeconds: 20,
			FailureThreshold:    3,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(8080)},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 15,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(8080)},
			},
		},
	}

	return
}

func (dashboard DashboardManager) newInitDashboardContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {
	clusterDomain :=cluster.GetClusterDomain()
	user, pwd := resources.GetClusterUser(cluster)
	env := dashboard.newDashboardEnv(serviceName, clusterDomain, user, pwd, cluster)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}
	container = corev1.Container{
		Name:            DashboardContainerName + "-init",
		Env:             env,
		Command:         []string{"sh", "scripts/prestart.sh"},
		WorkingDir:      "/app",
		Image:           cluster.Spec.Dashboard.Image,
		Resources:       cluster.Spec.Dashboard.Resources,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      resources.GreatdbPvcDataName,
				MountPath: dashboardDataMountPath,
			},
		},
	}

	return
}

func (dashboard DashboardManager) newDashboardEnv(serviceName, clusterDomain, user, pwd string, cluster *v1alpha1.GreatDBPaxos) (env []corev1.EnvVar) {
	monitorRemoteWrite := cluster.Spec.Dashboard.Config["monitorRemoteWrite"]
	defaultMetadata := cluster.Spec.Dashboard.Config["defaultMetadata"]
	permissionCheckUrl := cluster.Spec.Dashboard.Config["permissionCheckUrl"]
	lokiRemoteStorage := cluster.Spec.Dashboard.Config["lokiRemoteStorage"]
	if !strings.HasPrefix(permissionCheckUrl, "http") {
		permissionCheckUrl = "http://localhost:8099/gdbc/api/v1/cluster/init_cluster/verifyPermission"
	}

	metaDBHost := ""
	if defaultMetadata != "local" {
		metaDBHost = cluster.Name + string(service.GreatDBServiceWrite) + fmt.Sprintf(":%d", cluster.Spec.Port)
	}

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
			Name:  "FQDN",
			Value: "$(PODNAME).$(SERVICE_NAME).$(NAMESPACE).svc." + clusterDomain,
		},
		{
			Name:  "ADM_ADDRESS",
			Value: "127.0.0.1",
		},
		{
			Name:  "DAS_ADDRESS",
			Value: "127.0.0.1",
		},
		{
			Name:  "ADM_WEB_PORT",
			Value: "8080",
		},
		{
			Name:  "MONITOR_DATA_MAXIMUM_TIME",
			Value: "16d",
		},
		{
			Name:  "MONITOR_DATA_MAXIMUM_SIZE",
			Value: "20GB",
		},
		{
			Name:  "MAX_WORKERS",
			Value: "2",
		},
		{
			Name:  "DAS_MAX_WORKERS",
			Value: "2",
		},
		{
			Name:  "JXJK_PERMISSION_CHECK_URL",
			Value: permissionCheckUrl,
		},
		{
			Name:  "INTERNAL_ADM_WEB_PORT",
			Value: "8099",
		},
		{
			Name:  "MONITOR_REMOTE_WRITE",
			Value: monitorRemoteWrite,
		},
		{
			Name:  "LOKI_REMOTE_STORAGE",
			Value: lokiRemoteStorage,
		},
		{
			Name:  "ADM_METADB_USER",
			Value: user,
		},
		{
			Name:  "ADM_METADB_PASSWORD",
			Value: pwd,
		},
		{
			Name:  "ADM_METADB_HOST",
			Value: metaDBHost,
		},
		{
			Name:  "ADM_METADB_DBNAME",
			Value: "dbscale_dashboard",
		},
		{
			Name:  resources.ClusterUserKey,
			Value: user,
		},
		{
			Name:  resources.ClusterUserPasswordKey,
			Value: pwd,
		},
	}
	return
}

func (dashboard DashboardManager) newVolumes( cluster *v1alpha1.GreatDBPaxos) (volumes []corev1.Volume) {
	if cluster.Spec.Dashboard.PersistentVolumeClaimSpec.Resources.Requests.Storage().IsZero() {
		volumes = []corev1.Volume{
			{
				Name: resources.GreatdbPvcDataName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}
	}
	return
}

func (dashboard DashboardManager) updateDashboard(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(podIns.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		_, err := dashboard.Client.KubeClientset.CoreV1().Pods(podIns.Namespace).Patch(
			context.TODO(), podIns.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Reason(err).Errorf("failed to delete finalizers of pod %s/%s", podIns.Namespace, podIns.Name)
		}
		return nil
	}

	needUpdate := false
	// update labels
	if up := dashboard.updateLabels(podIns, cluster); up {
		needUpdate = true
	}

	// update annotations
	if up := dashboard.updateAnnotations(podIns, cluster); up {
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

		err := dashboard.updatePod(podIns)
		if err != nil {
			return err
		}

	}

	return nil
}



func (dashboard DashboardManager) updatePod(pod *corev1.Pod) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := dashboard.Client.KubeClientset.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		upPod, err1 := dashboard.Lister.PodLister.Pods(pod.Namespace).Get(pod.Name)
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

func (dashboard DashboardManager) updateLabels(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	labels := resources.MegerLabels(cluster.Spec.Labels, dashboard.GetLabels(cluster.Name))
	for key, value := range labels {
		if v, ok := pod.Labels[key]; !ok || v != value {
			pod.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (dashboard DashboardManager) updateAnnotations(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos) bool {
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