package greatdbpaxos

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/resources"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// pauseGreatdb Whether to pause the return instance
func (great GreatDBManager) upgradeGreatDB(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {
	if cluster.Status.Phase != v1alpha1.GreatDBPaxosReady && cluster.Status.Phase != v1alpha1.GreatDBPaxosUpgrade {
		return nil
	}

	if !podIns.DeletionTimestamp.IsZero() {
		return nil
	}

	if cluster.Status.UpgradeMember.Upgrading == nil {
		cluster.Status.UpgradeMember.Upgrading = make(map[string]string, 0)
	}
	if cluster.Status.UpgradeMember.Upgraded == nil {
		cluster.Status.UpgradeMember.Upgraded = make(map[string]string, 0)
	}

	err := great.upgradeInstance(cluster, podIns)
	if err != nil {
		return err
	}

	if len(cluster.Status.UpgradeMember.Upgrading) > 0 {
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosUpgrade, "")
	}

	return nil
}

// Pause successfully returns true
func (great GreatDBManager) upgradeInstance(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	needUpgrade := false

	for i, container := range podIns.Spec.Containers {
		if container.Name == GreatDBContainerName {
			if container.Image != cluster.Spec.Image {
				podIns.Spec.Containers[i].Image = cluster.Spec.Image
				needUpgrade = true
			}

		}
		if container.Name == GreatDBAgentContainerName {
			if container.Image != cluster.Spec.Image {
				podIns.Spec.Containers[i].Image = cluster.Spec.Image
				needUpgrade = true
			}

		}
	}

	if value, ok := cluster.Status.UpgradeMember.Upgrading[podIns.Name]; ok && !needUpgrade {
		// Upgrading requires at least 30 seconds before continuing to determine
		t := resources.StringToTime(value)
		if resources.GetNowTime().Sub(t) < time.Second*30 {
			return nil
		}

		for _, cond := range podIns.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				for _, member := range cluster.Status.Member {

					if member.Name == podIns.Name && member.Type == v1alpha1.MemberStatusOnline &&
						(t.Sub(member.LastTransitionTime.Time) < 0 || t.Sub(podIns.CreationTimestamp.Time) < 0) {
						cluster.Status.UpgradeMember.Upgraded[podIns.Name] = resources.GetNowTimeToString()
						delete(cluster.Status.UpgradeMember.Upgrading, podIns.Name)
						break
					}
				}
				break
			}
		}
		return nil
	}

	if !needUpgrade && len(cluster.Status.UpgradeMember.Upgraded) > 0 {
		cluster.Status.UpgradeMember.Upgraded[podIns.Name] = resources.GetNowTimeToString()
	}

	if great.upgradeClusterEnds(cluster) {
		cluster.Status.UpgradeMember.Upgraded = make(map[string]string, 0)
		cluster.Status.UpgradeMember.Upgrading = make(map[string]string, 0)
		return nil
	}

	if !needUpgrade {
		return nil
	}

	if _, ok := cluster.Status.UpgradeMember.Upgraded[podIns.Name]; ok {
		return nil
	}

	// Restart according to strategy
	switch cluster.Spec.UpgradeStrategy {
	case v1alpha1.AllUpgrade:
		err := great.updatePod(podIns)
		if err != nil {
			return err
		}

	case v1alpha1.RollingUpgrade:

		if len(cluster.Status.UpgradeMember.Upgrading) > 0 {
			_, ok := cluster.Status.UpgradeMember.Upgrading[podIns.Name]
			if !ok {
				return nil
			}

		}
		diag := great.ProbeStatus(cluster)
		memberList := diag.OnlineMembers
		canUpgrade := false
		secondaryAllUpgrade := true
		exist := false
		for _, member := range memberList {
			splitName := strings.Split(member.MemberHost, ".")
			name := member.MemberHost
			if len(splitName) > 0 {
				name = splitName[0]
			}
			if name == podIns.Name {
				exist = true
				if member.Role != string(v1alpha1.MemberRolePrimary) {
					canUpgrade = true
					break
				}
			}

			if member.Role == string(v1alpha1.MemberRoleSecondary) {
				if _, ok := cluster.Status.UpgradeMember.Upgraded[name]; !ok {
					secondaryAllUpgrade = false
				}
			}

		}

		if !canUpgrade && !secondaryAllUpgrade && exist {
			return nil
		}

		err := great.updatePod(podIns)
		if err != nil {
			return err
		}

	}
	cluster.Status.UpgradeMember.Upgrading[podIns.Name] = resources.GetNowTimeToString()

	return nil

}

func (GreatDBManager) upgradeClusterEnds(cluster *v1alpha1.GreatDBPaxos) bool {
	end := true
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}

		if _, ok := cluster.Status.UpgradeMember.Upgraded[member.Name]; !ok {
			end = false
		}
	}

	return end
}
