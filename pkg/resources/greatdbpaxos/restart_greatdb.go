package greatdbpaxos

import (
	"context"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"time"

	dblog "greatdb-operator/pkg/utils/log"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (great GreatDBManager) restartGreatDB(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) (err error) {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosReady && cluster.Status.Phase != v1alpha1.GreatDBPaxosRestart {
		return nil
	}

	if cluster.Spec.Restart == nil {
		cluster.Spec.Restart = &v1alpha1.RestartGreatDB{}
	}

	if cluster.Status.RestartMember.Restarting == nil {
		cluster.Status.RestartMember.Restarting = make(map[string]string, 0)
	}
	if cluster.Status.RestartMember.Restarted == nil {
		cluster.Status.RestartMember.Restarted = make(map[string]string, 0)
	}

	if !cluster.Spec.Restart.Enable && len(cluster.Status.RestartMember.Restarting) == 0 {
		return
	}

	if cluster.Spec.Restart.Mode == v1alpha1.ClusterRestart {
		return great.restartCluster(cluster, podIns)
	}

	err = great.restartInstance(cluster, podIns)
	if err != nil {
		return err
	}

	if len(cluster.Status.UpgradeMember.Upgrading) > 0 {
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosRestart, "")
	}

	return nil
}

func (great GreatDBManager) restartInstance(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if value, ok := cluster.Status.RestartMember.Restarting[podIns.Name]; ok {
		// Restarting requires at least 30 seconds before continuing to determine
		t := StringToTime(value)
		if time.Now().Local().Sub(t) < time.Second*30 {
			return nil
		}
		for _, member := range cluster.Status.Member {
			if member.Name == podIns.Name && member.Type == v1alpha1.MemberStatusOnline && t.Sub(member.LastTransitionTime.Time) < 0 {
				cluster.Status.RestartMember.Restarted[podIns.Name] = GetNowTime()
				delete(cluster.Status.RestartMember.Restarting, podIns.Name)
				break
			}
		}
		return nil
	}

	needRestart := false
	endRestart := true
	for _, ins := range cluster.Spec.Restart.Instances {
		if ins == podIns.Name {
			needRestart = true
		}
		if _, ok := cluster.Status.RestartMember.Restarted[ins]; !ok {
			endRestart = false
		}
	}

	if endRestart && len(cluster.Status.RestartMember.Restarting) == 0 && len(cluster.Status.RestartMember.Restarted) != 0 {
		cluster.Status.RestartMember.Restarted = make(map[string]string, 0)
		cluster.Status.RestartMember.Restarting = make(map[string]string, 0)
		cluster.Spec.Restart.Enable = false
		return nil
	}

	if !needRestart {
		return nil
	}

	if _, ok := cluster.Status.RestartMember.Restarted[podIns.Name]; ok {
		return nil
	}
	if !podIns.DeletionTimestamp.IsZero() {
		cluster.Status.RestartMember.Restarting[podIns.Name] = GetNowTime()
		return nil
	}

	// Restart according to strategy
	switch cluster.Spec.Restart.Strategy {
	case v1alpha1.AllRestart:
		err := great.deletePod(cluster.Namespace, podIns.Name)
		if err != nil {
			return err
		}

	case v1alpha1.RollingRestart:
		if len(cluster.Status.RestartMember.Restarting) > 0 {
			return nil
		}
		err := great.deletePod(cluster.Namespace, podIns.Name)
		if err != nil {
			return err
		}

	}
	cluster.Status.RestartMember.Restarting[podIns.Name] = GetNowTime()

	return nil

}

func (great GreatDBManager) restartCluster(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if value, ok := cluster.Status.RestartMember.Restarting[podIns.Name]; ok {
		// Restarting requires at least 30 seconds before continuing to determine
		t := StringToTime(value)

		if time.Now().Local().Sub(t) < time.Second*30 {
			return nil
		}
		for _, member := range cluster.Status.Member {
			if member.Name == podIns.Name && member.Type == v1alpha1.MemberStatusReady && t.Sub(member.LastTransitionTime.Time) < 0 {
				cluster.Status.RestartMember.Restarted[podIns.Name] = GetNowTime()
				delete(cluster.Status.RestartMember.Restarting, podIns.Name)
				break
			}
		}
		return nil
	}

	if great.restartClusterEnds(cluster) {
		cluster.Status.RestartMember.Restarted = make(map[string]string, 0)
		cluster.Status.RestartMember.Restarting = make(map[string]string, 0)
		cluster.Spec.Restart.Enable = false
		return nil
	}

	if !podIns.DeletionTimestamp.IsZero() {
		cluster.Status.RestartMember.Restarting[podIns.Name] = GetNowTime()
		return nil
	}

	// Restart according to strategy
	switch cluster.Spec.Restart.Strategy {
	case v1alpha1.AllRestart:
		err := great.deletePod(cluster.Namespace, podIns.Name)
		if err != nil {
			return err
		}

	case v1alpha1.RollingRestart:
		if len(cluster.Status.RestartMember.Restarting) > 0 {
			return nil
		}
		err := great.deletePod(cluster.Namespace, podIns.Name)
		if err != nil {
			return err
		}

	}
	cluster.Status.RestartMember.Restarting[podIns.Name] = GetNowTime()

	return nil

}

func (great GreatDBManager) deletePod(ns, name string) error {
	patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
	pods, err := great.Client.KubeClientset.CoreV1().Pods(ns).Patch(
		context.TODO(), name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		dblog.Log.Reason(err).Errorf("failed to delete finalizers of pods  %s/%s,", ns, name)
		return err
	}
	if !pods.DeletionTimestamp.IsZero() {
		return nil
	}

	err = great.Client.KubeClientset.CoreV1().Pods(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		dblog.Log.Reason(err).Errorf("failed to restart  pods  %s/%s,", ns, name)
		return err
	}

	return nil

}

func (GreatDBManager) restartClusterEnds(cluster *v1alpha1.GreatDBPaxos) bool {
	end := true
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}

		if _, ok := cluster.Status.RestartMember.Restarted[member.Name]; !ok {
			end = false
		}
	}

	return end
}
