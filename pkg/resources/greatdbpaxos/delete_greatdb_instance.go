package greatdbpaxos

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// pauseGreatdb Whether to pause the return instance
func (great GreatDBManager) deleteGreatDB(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

	if cluster.Spec.Delete == nil {
		cluster.Spec.Delete = &v1alpha1.DeleteInstance{}
	}

	return great.deleteInstance(cluster, member)

}

// Pause successfully returns true
func (great GreatDBManager) deleteInstance(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

	needDel := false
	for _, insName := range cluster.Spec.Delete.Instances {
		if insName == member.Name {
			needDel = true
			break
		}
	}

	if !needDel {
		return false, nil
	}

	err := great.deletePod(cluster.Namespace, member.Name)
	if err != nil {
		return false, err
	}

	if cluster.Spec.Delete.CleanPvc {
		pvc, err := great.Lister.PvcLister.PersistentVolumeClaims(cluster.Namespace).Get(member.PvcName)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				return false, err
			}
		} else {
			err = great.Deletepvc(pvc)
			if err != nil {
				return false, err
			}
		}

	}

	// remove
	great.removeMember(cluster, member.Name)

	return true, nil

}

func (GreatDBManager) removeMember(cluster *v1alpha1.GreatDBPaxos, name string) {
	memberList := make([]v1alpha1.MemberCondition, 0)

	for _, member := range cluster.Status.Member {
		if member.Name == name {
			continue
		}
		memberList = append(memberList, member)
	}
	cluster.Status.Member = memberList
}
