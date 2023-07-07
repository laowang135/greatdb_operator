package greatdbpaxos

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"

	greatresources "greatdb-operator/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// SetDefaultFields  Set the default configuration of cluster objects
func SetDefaultFields(cluster *v1alpha1.GreatDBPaxos) bool {
	if !cluster.DeletionTimestamp.IsZero() {
		return false
	}
	update := false
	if cluster.Spec.ImagePullPolicy == "" {
		update = true
		cluster.Spec.ImagePullPolicy = corev1.PullIfNotPresent
	}

	if cluster.Spec.ClusterDomain == "" {
		cluster.Spec.ClusterDomain = greatresources.DefaultClusterDomain
	}

	// set GreatDb component
	if SetGreatDB(cluster) {
		update = true
	}
	if SetGreatDBAgent(cluster) {
		update = true
	}

	// set metadata
	if SetMeta(cluster) {
		update = true
	}

	if SetSecurityContext(cluster) {
		update = true
	}
	return update
}

// SetMeta Configuration Label、 annotations and ownerReferences
func SetMeta(cluster *v1alpha1.GreatDBPaxos) bool {
	update := false

	// set Label"k8s.io/apimachinery/pkg/api/resource"
	if cluster.Labels == nil {
		cluster.Labels = make(map[string]string)
	}

	if _, ok := cluster.Labels[greatresources.AppKubeNameLabelKey]; !ok {
		cluster.Labels[greatresources.AppKubeNameLabelKey] = greatresources.AppKubeNameLabelValue
		update = true
	}

	if _, ok := cluster.Labels[greatresources.AppkubeManagedByLabelKey]; !ok {
		update = true
		cluster.Labels[greatresources.AppkubeManagedByLabelKey] = greatresources.AppkubeManagedByLabelValue
	}

	if cluster.Finalizers == nil {
		cluster.Finalizers = make([]string, 0)
	}
	existFinalizers := false
	for _, v := range cluster.Finalizers {
		if v == greatresources.FinalizersGreatDBCluster {
			existFinalizers = true
		}
	}
	if !existFinalizers {
		update = true
		cluster.Finalizers = append(cluster.Finalizers, greatresources.FinalizersGreatDBCluster)
	}

	return update

}

func SetGreatDB(cluster *v1alpha1.GreatDBPaxos) bool {
	update := false

	if cluster.Spec.Service.Type == "" {
		cluster.Spec.Service.Type = corev1.ServiceTypeClusterIP
	}

	// restart

	if cluster.Spec.Restart == nil {
		cluster.Spec.Restart = &v1alpha1.RestartGreatDB{}
	}

	if cluster.Spec.Restart.Mode != v1alpha1.ClusterRestart {
		cluster.Spec.Restart.Mode = v1alpha1.InsRestart
	}
	if cluster.Spec.Restart.Strategy != v1alpha1.AllRestart {
		cluster.Spec.Restart.Strategy = v1alpha1.RollingRestart
	}

	if cluster.Spec.UpgradeStrategy != v1alpha1.AllUpgrade {
		cluster.Spec.UpgradeStrategy = v1alpha1.RollingUpgrade
	}

	if cluster.Spec.Scaling == nil {
		cluster.Spec.Scaling = &v1alpha1.Scaling{
			ScaleIn: v1alpha1.ScaleIn{
				Strategy: v1alpha1.ScaleInStrategyIndex,
			},
		}
	}

	// TODO DEBUG
	if cluster.Spec.Scaling.ScaleOut.Source == "" {
		cluster.Spec.Scaling.ScaleOut.Source = v1alpha1.ScaleOutSourceBackup
	}

	if cluster.Spec.Instances < 2 {
		update = true
		cluster.Spec.Instances = 2
	}

	if cluster.Spec.Port == 0 {
		cluster.Spec.Port = 3306
	}
	maxUnavailable := intstr.FromInt(0)
	if cluster.Spec.FailOver.MaxUnavailable == nil {
		cluster.Spec.FailOver.MaxUnavailable = &maxUnavailable
		update = true
	}

	if cluster.Spec.FailOver.Enable == nil {
		update = true
		cluster.Spec.FailOver.Enable = &update
	}

	if cluster.Spec.FailOver.AutoScaleiIn == nil {
		update = true
		scaleIn := false
		cluster.Spec.FailOver.AutoScaleiIn = &scaleIn
	}

	if cluster.Spec.Resources.Requests == nil {
		cluster.Spec.Resources.Requests = make(corev1.ResourceList)
	}

	if cluster.Spec.Resources.Limits == nil {
		cluster.Spec.Resources.Limits = make(corev1.ResourceList)
	}

	// Check the cpu memory and set the minimum value
	if cluster.Spec.Resources.Requests.Cpu().IsZero() {
		update = true
		cluster.Spec.Resources.Requests[corev1.ResourceCPU] = *resource.NewQuantity(3, resource.BinarySI)
	}

	if cluster.Spec.Resources.Limits.Cpu().Cmp(*cluster.Spec.Resources.Requests.Cpu()) == -1 {
		update = true
		cluster.Spec.Resources.Limits[corev1.ResourceCPU] = cluster.Spec.Resources.Requests[corev1.ResourceCPU]
	}

	if cluster.Spec.Resources.Requests.Memory().IsZero() {
		update = true
		cluster.Spec.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("3Gi")
	}

	if cluster.Spec.Resources.Limits.Memory().Cmp(*cluster.Spec.Resources.Requests.Memory()) == -1 {
		update = true
		cluster.Spec.Resources.Limits[corev1.ResourceMemory] = cluster.Spec.Resources.Requests[corev1.ResourceMemory]
	}

	pvc := cluster.Spec.VolumeClaimTemplates
	size := pvc.Resources.Requests.Storage().String()

	pvc, change := SetVolumeClaimTemplates(size, pvc.StorageClassName, pvc.AccessModes)
	if change {
		update = true
		cluster.Spec.VolumeClaimTemplates = pvc
	}

	return update

}

func SetVolumeClaimTemplates(size string, storageClassName *string, AccessMode []corev1.PersistentVolumeAccessMode) (corev1.PersistentVolumeClaimSpec, bool) {

	update := false
	if size == "0" || size == "" {
		size = "10Gi"
		update = true
	}

	if len(AccessMode) == 0 {
		AccessMode = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		update = true
	}

	volume := corev1.PersistentVolumeClaimSpec{
		AccessModes:      AccessMode,
		StorageClassName: storageClassName,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			}},
	}
	return volume, update

}

func SetSecurityContext(cluster *v1alpha1.GreatDBPaxos) bool {
	// var runAsGroup  int64 = 1000
	var runAsUser int64 = 1000
	var fsGroup int64 = 1000
	update := false

	// if config.ServerVersion >= "1.14.0" {
	// 	if cluster.Spec.PodSecurityContext.FSGroupChangePolicy == nil {
	// 		policy := corev1.FSGroupChangeOnRootMismatch
	// 		cluster.Spec.PodSecurityContext.FSGroupChangePolicy = &policy
	// 		update = true
	// 	}
	// }

	// fsgroup
	if cluster.Spec.PodSecurityContext.RunAsGroup == nil && cluster.Spec.PodSecurityContext.FSGroup == nil {
		cluster.Spec.PodSecurityContext.FSGroup = &fsGroup
		return true
	}

	if cluster.Spec.PodSecurityContext.RunAsGroup != nil && *cluster.Spec.PodSecurityContext.RunAsGroup != *cluster.Spec.PodSecurityContext.FSGroup {
		cluster.Spec.PodSecurityContext.FSGroup = cluster.Spec.PodSecurityContext.RunAsGroup
		update = true
	}

	if cluster.Spec.PodSecurityContext.RunAsGroup == nil && *cluster.Spec.PodSecurityContext.FSGroup != fsGroup {
		cluster.Spec.PodSecurityContext.FSGroup = &fsGroup
		update = true
	}

	// runAsUser
	if cluster.Spec.PodSecurityContext.RunAsGroup == nil {
		cluster.Spec.PodSecurityContext.RunAsUser = &runAsUser
		update = true
	}

	return update

}

func SetGreatDBAgent(cluster *v1alpha1.GreatDBPaxos) bool {
	update := false

	if cluster.Spec.Backup.Enable == nil {
		enable := true
		cluster.Spec.Backup.Enable = &enable
		update = true
	}

	if !*cluster.Spec.Backup.Enable {
		return update
	}

	if cluster.Spec.Backup.Resources.Requests == nil {
		cluster.Spec.Backup.Resources.Requests = make(corev1.ResourceList)
	}

	if cluster.Spec.Backup.Resources.Limits == nil {
		cluster.Spec.Backup.Resources.Limits = make(corev1.ResourceList)
	}

	// Check the cpu memory and set the minimum value
	if cluster.Spec.Backup.Resources.Requests.Cpu().IsZero() {
		update = true
		cluster.Spec.Backup.Resources.Requests[corev1.ResourceCPU] = *resource.NewQuantity(2, resource.BinarySI)
	}

	if cluster.Spec.Backup.Resources.Limits.Cpu().Cmp(*cluster.Spec.Backup.Resources.Requests.Cpu()) == -1 {
		update = true
		cluster.Spec.Backup.Resources.Limits[corev1.ResourceCPU] = cluster.Spec.Backup.Resources.Requests[corev1.ResourceCPU]
	}

	if cluster.Spec.Backup.Resources.Requests.Memory().IsZero() {
		update = true
		cluster.Spec.Backup.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("2Gi")
	}

	if cluster.Spec.Backup.Resources.Limits.Memory().Cmp(*cluster.Spec.Backup.Resources.Requests.Memory()) == -1 {
		update = true
		cluster.Spec.Backup.Resources.Limits[corev1.ResourceMemory] = cluster.Spec.Backup.Resources.Requests[corev1.ResourceMemory]
	}

	return update
}
