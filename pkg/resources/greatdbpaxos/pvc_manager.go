package greatdbpaxos

import (
	"context"
	"encoding/json"
	"fmt"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/resources"
	dblog "greatdb-operator/pkg/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// dblog "greatdb-operator/pkg/utils/log"
)

func (great GreatDBManager) SyncPvc(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {

	if member.PvcName == "" {
		member.PvcName = member.Name
	}

	pvc, err := great.Lister.PvcLister.PersistentVolumeClaims(cluster.Namespace).Get(member.PvcName)
	if err != nil {

		if k8serrors.IsNotFound(err) {
			return great.CreatePvc(cluster, member)
		}
		dblog.Log.Reason(err).Errorf("failed to lister pvc %s/%s", cluster.Namespace, member.PvcName)

	}

	err = great.UpdatePvc(cluster, pvc)
	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to update pvc %s%s", pvc.Namespace, pvc.Name)
		return err
	}
	return nil

}

func (great GreatDBManager) CreatePvc(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {

	if member.PvcName == "" {
		member.PvcName = member.Name
	}

	pvc := great.NewPvc(cluster, member)

	_, err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(cluster.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(pvc.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			_, err = great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(cluster.Namespace).Patch(
				context.TODO(), pvc.Name, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of pod, message: %s", err.Error())
				return err
			}
			return nil
		}
	}
	dblog.Log.Infof("successfully created PVC %s/%s", pvc.Namespace, pvc.Name)

	return nil

}

// GetPvcLabels  Return to the default label settings
func (great GreatDBManager) GetPvcLabels(name, podName string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = name
	labels[resources.AppKubePodLabelKey] = podName
	return

}

func (great GreatDBManager) NewPvc(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (pvc *corev1.PersistentVolumeClaim) {

	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            member.PvcName,
			Namespace:       cluster.Namespace,
			Labels:          great.GetPvcLabels(cluster.Name, member.Name),
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: cluster.Spec.VolumeClaimTemplates,
	}
	return

}

func (great GreatDBManager) UpdatePvc(cluster *v1alpha1.GreatDBPaxos, pvc *corev1.PersistentVolumeClaim) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(pvc.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		_, err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Patch(
			context.TODO(), pvc.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of pvc %s/%s,message: %s", pvc.Namespace, pvc.Name, err.Error())
		}

		return nil

	}

	return nil

}
