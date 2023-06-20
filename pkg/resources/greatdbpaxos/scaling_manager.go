package greatdbpaxos

import (
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/resources/internal"
	dblog "greatdb-operator/pkg/utils/log"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

func (great GreatDBManager) Scale(cluster *v1alpha1.GreatDBPaxos) error {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosReady && cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleIn &&
		cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleOut && cluster.Status.Phase != v1alpha1.GreatDBPaxosRepair {
		return nil
	}

	if cluster.Status.TargetInstances != cluster.Spec.Instances && (cluster.Status.Phase == v1alpha1.GreatDBPaxosReady ||
		cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleOut || cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleIn) {
		cluster.Status.TargetInstances = cluster.Spec.Instances
	}

	if err := great.ScaleOut(cluster); err != nil {
		return err
	}

	if err := great.ScaleIn(cluster); err != nil {
		return err
	}

	return nil

}

func (great GreatDBManager) ScaleOut(cluster *v1alpha1.GreatDBPaxos) error {

	if cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleIn {
		return nil
	}

	// Only one node can be expanded at a time
	if cluster.Status.TargetInstances > cluster.Status.CurrentInstances {
		//determine whether the instance has completed expansion (joining the cluster)
		member := great.GetMinOrMaxIndexMember(cluster, true)
		// Waiting for the expansion of the previous instance to end
		if member.Type != v1alpha1.MemberStatusOnline && cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleOut {
			return nil
		}

		if cluster.Status.Phase != v1alpha1.GreatDBPaxosRepair && cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleOut {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosScaleOut, "")
		}

		num := len(cluster.Status.Member)

		if num >= int(cluster.Status.TargetInstances) {
			return nil
		}

		index := GetNextIndex(cluster.Status.Member)
		name := fmt.Sprintf("%s%s-%d", cluster.Name, resources.ComponentGreatDBSuffix, index)
		cluster.Status.Member = append(cluster.Status.Member, v1alpha1.MemberCondition{
			Name:       name,
			Index:      index,
			CreateType: v1alpha1.ScaleCreateMember,
			PvcName:    name,
		})
		cluster.Status.CurrentInstances += 1

		groupSeed := ""
		hosts := great.getGreatdbServiceClientUri(cluster)

		for _, host := range hosts {
			groupSeed += fmt.Sprintf("%s:%d,", host, resources.GroupPort)
		}
		groupSeed = strings.TrimSuffix(groupSeed, ",")

		// return great.SetVariableGroupSeeds(cluster,hosts,groupSeed)

		// TODO DEBUG
		hostList := make([]string, 0)
		for _, member := range cluster.Status.Member {
			if member.Name == name {
				continue
			}

			if member.Type == v1alpha1.MemberStatusPause || member.Type == v1alpha1.MemberStatusRestart {
				continue
			}

			host := resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.GetClusterDomain())
			hostList = append(hostList, host)
		}
		return great.SetVariableGroupSeeds(cluster, hostList, groupSeed)

	} else {
		member := great.GetMinOrMaxIndexMember(cluster, true)
		if cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleOut && member.Type == v1alpha1.MemberStatusOnline {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	}

	return nil

}

func (great GreatDBManager) ScaleIn(cluster *v1alpha1.GreatDBPaxos) error {
	// Only one node can be expanded at a time
	if cluster.Status.TargetInstances < cluster.Status.CurrentInstances {
		if cluster.Status.Phase != v1alpha1.GreatDBPaxosRepair && cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleIn {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosScaleIn, "")
		}
		num := len(cluster.Status.Member)

		if num < int(cluster.Status.TargetInstances) {
			return nil
		}
		// Waiting for the previous instance to shrink to an end
		if cluster.Status.ScaleInMember != "" {
			_, err := great.Lister.PodLister.Pods(cluster.Namespace).Get(cluster.Status.ScaleInMember)
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil
			}
			cluster.Status.ScaleInMember = ""
		}
		cluster.Status.CurrentInstances -= 1

		member := great.getScaleInMember(cluster)
		great.ScaleInMember(cluster, member.Name)
		err := great.deletePod(cluster.Namespace, member.Name)
		if err != nil {
			return err
		}
		cluster.Status.ScaleInMember = member.Name

		groupSeed := ""
		hosts := great.getGreatdbServiceClientUri(cluster)

		for _, host := range hosts {
			groupSeed += fmt.Sprintf("%s:%d,", host, resources.GroupPort)
		}
		groupSeed = strings.TrimSuffix(groupSeed, ",")

		// return great.SetVariableGroupSeeds(cluster, hosts,groupSeed)

		// TODO DEBUG
		hostList := make([]string, 0)
		for _, member := range cluster.Status.Member {

			if member.Type == v1alpha1.MemberStatusPause || member.Type == v1alpha1.MemberStatusRestart {
				continue
			}

			host := resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.GetClusterDomain())
			hostList = append(hostList, host)
		}
		return great.SetVariableGroupSeeds(cluster, hostList, groupSeed)

	} else {
		if cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleIn {
			cluster.Status.ScaleInMember = ""
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}
	}

	return nil

}

func (great GreatDBManager) getGreatdbServiceClientUri(cluster *v1alpha1.GreatDBPaxos) (uris []string) {

	for _, member := range cluster.Status.Member {

		if member.Address != "" {
			uris = append(uris, member.Address)
			continue
		}
		svcName := cluster.Name + resources.ComponentGreatDBSuffix
		host := fmt.Sprintf("%s.%s.%s.svc.%s", member.Name, svcName, cluster.Namespace, cluster.Spec.ClusterDomain)
		// TODO Debug
		// host := resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
		uris = append(uris, host)
	}

	return uris
}

func (great GreatDBManager) SetVariableGroupSeeds(cluster *v1alpha1.GreatDBPaxos, hostList []string, groupSeed string) error {

	sql := fmt.Sprintf("SET GLOBAL group_replication_group_seeds='%s'", groupSeed)
	client := internal.NewDBClient()
	user, password := resources.GetClusterUser(cluster)
	for _, host := range hostList {

		err := client.Connect(user, password, host, int(cluster.Spec.Port), "mysql")
		if err != nil {
			dblog.Log.Reason(err).Error("failed to connect mysql")

			return err
		}

		err = client.Exec(sql)
		if err != nil {
			client.Close()
			dblog.Log.Reason(err).Errorf("failed to set variable %s on node group_replication_group_seeds", host)
			return err
		}
		client.Close()
	}
	return nil

}

func (great GreatDBManager) getScaleInMember(cluster *v1alpha1.GreatDBPaxos) v1alpha1.MemberCondition {

	var member v1alpha1.MemberCondition
	switch cluster.Spec.Scaling.ScaleIn.Strategy {
	case v1alpha1.ScaleInStrategyDefine:
	case v1alpha1.ScaleInStrategyFault:
	default:
		num := len(cluster.Status.Member)
		member = cluster.Status.Member[num-1]

	}

	return member

}

func (great GreatDBManager) ScaleInMember(cluster *v1alpha1.GreatDBPaxos, name string) {

	index := 0
	exist := false
	for i, member := range cluster.Status.Member {

		if member.Name == name {
			exist = true
			index = i
		}
	}
	if !exist {
		return
	}

	cluster.Status.Member = append(cluster.Status.Member[:index], cluster.Status.Member[index+1:]...)

}

func (great GreatDBManager) GetMinOrMaxIndexMember(cluster *v1alpha1.GreatDBPaxos, max bool) v1alpha1.MemberCondition {

	index := 0

	var member v1alpha1.MemberCondition
	for _, m := range cluster.Status.Member {
		if max {
			if m.Index >= index {
				member = m
			}
		} else {
			if m.Index <= index {
				member = m
			}
		}

	}
	return member

}
