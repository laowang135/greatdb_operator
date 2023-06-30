package greatdbpaxos

import (
	"context"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/utils/log"
	"reflect"

	"k8s.io/api/policy/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (greatdb GreatDBManager) createPDB(cluster *v1alpha1.GreatDBPaxos) error {

	pdb := greatdb.newPDB(cluster)
	_, err := greatdb.Client.KubeClientset.PolicyV1beta1().PodDisruptionBudgets(cluster.Namespace).Create(context.TODO(), pdb, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		log.Log.Reason(err).Errorf("failed to create pdb %s/%s", pdb.Namespace, pdb.Name)
		return err
	}

	return nil
}

func (greatdb GreatDBManager) newPDB(cluster *v1alpha1.GreatDBPaxos) *v1beta1.PodDisruptionBudget {

	labels := greatdb.GetLabels(cluster.Name)
	pdbSpec := v1beta1.PodDisruptionBudgetSpec{

		MaxUnavailable: cluster.Spec.FailOver.MaxUnavailable,
		Selector:       metav1.SetAsLabelSelector(labels),
	}
	pdbName := greatdb.getPDBName(cluster)

	if pdbSpec.Selector == nil {
		pdbSpec.Selector = metav1.SetAsLabelSelector(labels)
	}

	pdb := &v1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pdbName,
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)},
		},
		Spec: pdbSpec,
	}

	return pdb
}

func (greatdb GreatDBManager) updatePDB(cluster *v1alpha1.GreatDBPaxos, pdb *v1beta1.PodDisruptionBudget) error {
	labels := greatdb.GetLabels(cluster.Name)
	pdbSpec := v1beta1.PodDisruptionBudgetSpec{
		MaxUnavailable: cluster.Spec.FailOver.MaxUnavailable,
		Selector:       metav1.SetAsLabelSelector(labels),
	}

	if reflect.DeepEqual(pdbSpec, pdb.Spec) {
		return nil
	}
	pdb.Spec = pdbSpec
	_, err := greatdb.Client.KubeClientset.PolicyV1beta1().PodDisruptionBudgets(cluster.Namespace).Update(context.TODO(), pdb, metav1.UpdateOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		log.Log.Reason(err).Errorf("failed to update pdb %s/%s", pdb.Namespace, pdb.Name)
		return err
	}

	return nil
}

func (GreatDBManager) getPDBName(cluster *v1alpha1.GreatDBPaxos) string {
	return fmt.Sprintf("%s%s", cluster.Name, resources.ComponentGreatDBSuffix)
}
