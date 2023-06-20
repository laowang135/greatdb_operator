package greatdbpaxos

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
)

func (great GreatDBManager) Scale(cluster *v1alpha1.GreatDBPaxos) error {

	return nil

}

func (great GreatDBManager) ScaleOut(cluster *v1alpha1.GreatDBPaxos) error {

	if cluster.Status.TargetInstances > cluster.Status.CurrentInstances {

	}

	return nil

}

func (great GreatDBManager) ScaleIn(cluster *v1alpha1.GreatDBPaxos) error {

	return nil

}
