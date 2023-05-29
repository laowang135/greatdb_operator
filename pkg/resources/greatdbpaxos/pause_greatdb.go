package greatdbpaxos

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// pauseGreatdb Whether to pause the return instance
func (great GreatDBManager) pauseGreatDB(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

	if !great.needPause(cluster, member) {
		return false, nil
	}

	return great.pauseInstance(cluster, member)

}

// Pause successfully returns true
func (great GreatDBManager) pauseInstance(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

	pod, err := great.Lister.PodLister.Pods(cluster.Namespace).Get(member.Name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	if !pod.DeletionTimestamp.IsZero() {
		return true, err
	}

	err = great.deletePod(cluster.Namespace, member.Name)
	if err != nil {
		return false, err
	}
	return true, nil

}

func (GreatDBManager) needPause(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) bool {

	if !cluster.Spec.Pause.Enable {
		return false
	}

	if cluster.Spec.Pause.Mode == v1alpha1.ClusterPause {
		return true
	}

	for _, ins := range cluster.Spec.Pause.Instances {
		if ins == member.Name {
			return true
		}

	}
	return false
}

func (GreatDBManager) GetRunningMember(cluster *v1alpha1.GreatDBPaxos) int {

	num := 0
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}
		num++
	}

	return num
}
