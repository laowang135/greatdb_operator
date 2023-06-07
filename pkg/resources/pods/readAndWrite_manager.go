package pods

import (
	"context"
	"encoding/json"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/utils/log"

	"greatdb-operator/pkg/resources/greatdbpaxos"
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
	diag := greatdbpaxos.DiagnoseCluster(cluster, great.Lister)

	for _, ins := range diag.AllInstance {
		if err := great.updatePod(ns, ins.PodIns.Name, ins.Role, ins.State, cluster); err != nil {
			return err
		}
	}
	return nil

}

func (great ReadAndWriteManager) updatePod(ns, podName string, role string, status v1alpha1.MemberConditionType, cluster *v1alpha1.GreatDBPaxos) error {

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

func (great ReadAndWriteManager) updateLabels(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos, role string, status v1alpha1.MemberConditionType) (bool, string) {
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
