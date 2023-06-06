package pods

import (
	"context"
	"encoding/json"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/resources/internal"
	"strings"

	"greatdb-operator/pkg/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type ReadAndWriteManager struct {
	Client *deps.ClientSet
	Lister *deps.Listers
}

func (great ReadAndWriteManager) Sync(cluster *v1alpha1.GreatDBPaxos) error {

	if err := great.updateRole(cluster); err != nil {
		return err
	}

	return nil
}

func (great ReadAndWriteManager) updateRole(cluster *v1alpha1.GreatDBPaxos) error {

	ns := cluster.Namespace
	memberList, err := great.GetMemberList(cluster)
	if err != nil {
		return err
	}

	insStatusSet := make(map[string]v1alpha1.MemberCondition)

	for _, ser := range memberList {

		splitHost := strings.Split(ser.Host, ".")
		name := ""
		if len(splitHost) > 0 {
			name = splitHost[0]
		}

		insStatusSet[name] = v1alpha1.MemberCondition{
			Name: name,
			Type: v1alpha1.MemberConditionType(ser.State).Parse(),
			Role: v1alpha1.MemberRoleType(ser.Role).Parse(),
		}

	}
	var ins v1alpha1.MemberCondition
	for _, member := range cluster.Status.Member {

		ok := false
		if ins, ok = insStatusSet[member.Name]; !ok {
			ins = v1alpha1.MemberCondition{
				Role: v1alpha1.MemberRoleUnknown,
				Type: v1alpha1.MemberStatusFree,
			}
		}

		if ins.Type != v1alpha1.MemberStatusOnline && ins.Type != v1alpha1.MemberStatusRecovering {
			ins.Role = v1alpha1.MemberRoleUnknown
		}

		great.updatePod(ns, member.Name, ins.Role, ins.Type, cluster)

	}
	return nil

}

func (great ReadAndWriteManager) updatePod(ns, podName string, role v1alpha1.MemberRoleType, status v1alpha1.MemberConditionType, cluster *v1alpha1.GreatDBPaxos) error {

	pod, err := great.Lister.PodLister.Pods(ns).Get(podName)
	if err != nil {
		log.Log.Reason(err).Errorf("failed to lister pod %s/%s", ns, podName)
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	up, data := great.updateLabels(pod, cluster, role, status)
	if !up {
		return nil
	}

	_, err = great.Client.KubeClientset.CoreV1().Pods(ns).Patch(context.TODO(), podName, types.JSONPatchType, []byte(data), metav1.PatchOptions{})
	if err != nil {
		log.Log.Reason(err).Errorf("failed to update label of pod %s/%s", pod.Namespace, pod.Name)
		return err
	}

	return nil
}

func (great ReadAndWriteManager) GetMemberList(cluster *v1alpha1.GreatDBPaxos) ([]resources.PaxosMember, error) {

	ns := cluster.Namespace
	clusterName := cluster.Name
	clusterDomain := cluster.Spec.ClusterDomain
	port := int(cluster.Spec.Port)
	user, pwd := resources.GetClusterUser(cluster)

	sqlClient := internal.NewDBClient()
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}
		host := resources.GetInstanceFQDN(clusterName, member.Name, ns, clusterDomain)
		err := sqlClient.Connect(user, pwd, host, port, "mysql")
		if err != nil {
			log.Log.Reason(err).Error("connection_error")
			continue
		}
		memberList := make([]resources.PaxosMember, 0)

		err = sqlClient.Query(resources.QueryClusterMemberStatus, &memberList, resources.QueryClusterMemberFields)
		if err != nil {
			sqlClient.Close()
			log.Log.Reason(err).Errorf("failed to query cluster status")
			return memberList, err
		}
		sqlClient.Close()
		for _, status := range memberList {
			if status.State == string(v1alpha1.MemberStatusOnline) {
				return memberList, nil
			}
		}
	}
	log.Log.Errorf("Cluster %s/%s error", ns, clusterName)
	return nil, nil

}

func (great ReadAndWriteManager) updateLabels(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos, role v1alpha1.MemberRoleType, status v1alpha1.MemberConditionType) (bool, string) {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	newLabel := resources.MegerLabels(pod.Labels)

	labels := resources.MegerLabels(cluster.Spec.Labels, great.GetLabels(cluster.Name, string(role)))
	ready := resources.AppKubeServiceNotReady
	if status == v1alpha1.MemberStatusOnline {
		ready = resources.AppKubeServiceReady
	}
	labels[resources.AppKubeServiceReadyLabelKey] = ready

	for key, value := range labels {
		if v, ok := newLabel[key]; !ok || v != value {
			newLabel[key] = value
			needUpdate = true
		}
	}
	patch := ""
	if needUpdate {
		data, _ := json.Marshal(newLabel)
		patch = fmt.Sprintf(`[{"op":"add","path":"/metadata/labels","value":%s}]`, data)
	}

	return needUpdate, patch
}

// GetLabels  Return to the default label settings
func (great ReadAndWriteManager) GetLabels(clusterName, role string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = clusterName
	labels[resources.AppKubeGreatDBRoleLabelKey] = role
	return

}
