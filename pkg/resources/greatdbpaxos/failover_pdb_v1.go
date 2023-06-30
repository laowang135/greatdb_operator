package greatdbpaxos

import (
	"context"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/utils/log"
	"reflect"

	policyV1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (greatdb GreatDBManager) createV1PDB(cluster *v1alpha1.GreatDBPaxos) error {

	pdb := greatdb.newV1PDB(cluster)
	_, err := greatdb.Client.KubeClientset.PolicyV1().PodDisruptionBudgets(cluster.Namespace).Create(context.TODO(), pdb, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		log.Log.Reason(err).Errorf("failed to create pdb %s/%s", pdb.Namespace, pdb.Name)
		return err
	}

	return nil
}

func (greatdb GreatDBManager) newV1PDB(cluster *v1alpha1.GreatDBPaxos) *policyV1.PodDisruptionBudget {

	labels := greatdb.GetLabels(cluster.Name)
	pdbSpec := policyV1.PodDisruptionBudgetSpec{

		MaxUnavailable: cluster.Spec.FailOver.MaxUnavailable,
		Selector:       metav1.SetAsLabelSelector(labels),
	}
	pdbName := greatdb.getPDBName(cluster)

	if pdbSpec.Selector == nil {
		pdbSpec.Selector = metav1.SetAsLabelSelector(labels)
	}

	pdb := &policyV1.PodDisruptionBudget{
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

func (greatdb GreatDBManager) updateV1PDB(cluster *v1alpha1.GreatDBPaxos, pdb *policyV1.PodDisruptionBudget) error {
	labels := greatdb.GetLabels(cluster.Name)
	pdbSpec := policyV1.PodDisruptionBudgetSpec{
		MaxUnavailable: cluster.Spec.FailOver.MaxUnavailable,
		Selector:       metav1.SetAsLabelSelector(labels),
	}

	if reflect.DeepEqual(pdbSpec, pdb.Spec) {
		return nil
	}
	pdb.Spec = pdbSpec
	_, err := greatdb.Client.KubeClientset.PolicyV1().PodDisruptionBudgets(cluster.Namespace).Update(context.TODO(), pdb, metav1.UpdateOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		log.Log.Reason(err).Errorf("failed to update pdb %s/%s", pdb.Namespace, pdb.Name)
		return err
	}

	return nil
}
