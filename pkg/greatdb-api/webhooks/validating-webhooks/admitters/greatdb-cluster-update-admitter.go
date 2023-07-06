package admitters

import (
	"context"
	"fmt"
	"reflect"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/utils/log"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
)

type GreatDBClusterUpdateAdmitter struct {
	KubeClient    kubernetes.Interface
	GreatDBClient versioned.Interface
}

func (admit GreatDBClusterUpdateAdmitter) Admit(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	newCluster, oldCluster, err := utils.GetGreatDBClusterFromAdmissionReview(ar)
	if err != nil {
		return utils.ToAdmissionResponseError(err)
	}

	if !reflect.DeepEqual(newCluster, oldCluster) {
		causes := ValidatingGreatDBClusterUpdateSpec(k8sfield.NewPath("spec"), newCluster, oldCluster, admit.KubeClient, admit.GreatDBClient)
		return utils.NewAdmissionResponse(causes)
	}
	return utils.NewPassingAdmissionResponse()
}

func ValidatingGreatDBClusterUpdateSpec(field *k8sfield.Path, cluster, oldCluster *v1alpha1.GreatDBPaxos, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause
	causes = append(causes, ValidatingUpdateSecretName(field.Child("secretName"), cluster, oldCluster, kubeClient)...)
	causes = append(causes, ValidatingPriorityClassName(field.Child("priorityClassName"), cluster.Spec.PriorityClassName, kubeClient)...)
	causes = append(causes, ValidatingImagePullSecrets(field.Child("imagePullSecrets"), cluster.Namespace, cluster.Spec.ImagePullSecrets, kubeClient)...)
	causes = append(causes, ValidatingUpdateService(field.Child("service"), cluster, oldCluster, kubeClient)...)
	causes = append(causes, ValidatingUpdatePort(field.Child("port"), cluster, oldCluster)...)
	causes = append(causes, ValidatingCloneSource(field.Child("cloneSource"), cluster.Namespace, cluster.Spec.CloneSource, greatdbClient)...)
	return causes
}

func ValidatingUpdateSecretName(field *k8sfield.Path, cluster, oldCluster *v1alpha1.GreatDBPaxos, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if oldCluster.Spec.SecretName != "" && oldCluster.Spec.SecretName != cluster.Spec.SecretName {
		if oldCluster.Spec.SecretName != "" {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "prohibit modification",
				Field:   field.String(),
			})
		}
		return causes
	}
	secretName := cluster.Spec.SecretName
	ns := cluster.Namespace
	if secretName == "" {
		return causes
	}

	_, err := client.CoreV1().Secrets(ns).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil && k8serrors.IsNotFound(err) {
		log.Log.Error(err.Error())
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueNotFound,
			Message: fmt.Sprintf("secret %s/%s not exist", ns, secretName),
			Field:   field.String(),
		})
	}

	return causes
}

func ValidatingUpdatePort(field *k8sfield.Path, cluster, oldCluster *v1alpha1.GreatDBPaxos) []metav1.StatusCause {
	var causes []metav1.StatusCause
	if oldCluster.Spec.Port == 0 {
		return causes
	}

	if cluster.Spec.Port != oldCluster.Spec.Port {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Invalid value: %d: prohibit modification", cluster.Spec.Port),
			Field:   field.String(),
		})
	}

	return causes
}

func ValidatingUpdateService(field *k8sfield.Path, cluster, oldCluster *v1alpha1.GreatDBPaxos, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if cluster.Spec.Service.Type != corev1.ServiceTypeNodePort {
		return causes
	}

	if cluster.Spec.Service.ReadPort == 0 && cluster.Spec.Service.WritePort == 0 {
		return causes
	}

	if cluster.Spec.Service.ReadPort == cluster.Spec.Service.WritePort {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Invalid value: readPort【%d], writePort【%d】: Prohibit setting to the same value", cluster.Spec.Service.ReadPort, cluster.Spec.Service.WritePort),
			Field:   field.String(),
		})
	}

	var readEquation bool
	if cluster.Spec.Service.ReadPort == oldCluster.Spec.Service.ReadPort {
		readEquation = true
	}
	writeQeuation := false
	if cluster.Spec.Service.WritePort == oldCluster.Spec.Service.WritePort {
		writeQeuation = true
	}

	if readEquation && writeQeuation {
		return causes
	}

	if cluster.Spec.Service.ReadPort == 0 && cluster.Spec.Service.WritePort == 0 {
		return causes
	}

	serviceList, err := client.CoreV1().Services(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return causes
	}

	read := false
	if cluster.Spec.Service.ReadPort == 0 {
		read = true
	}
	write := false
	if cluster.Spec.Service.WritePort == 0 {
		write = true
	}
	for _, svc := range serviceList.Items {

		if read && write {
			break
		}
		if svc.Spec.Type != corev1.ServiceTypeNodePort {
			continue
		}

		for _, port := range svc.Spec.Ports {
			if !readEquation && port.NodePort == cluster.Spec.Service.ReadPort {
				read = true
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueInvalid,
					Message: fmt.Sprintf("Invalid value: %d: provided port is already allocated", cluster.Spec.Service.ReadPort),
					Field:   field.Child("readPort").String(),
				})
			}

			if !writeQeuation && port.NodePort == cluster.Spec.Service.WritePort {
				write = true
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueInvalid,
					Message: fmt.Sprintf("Invalid value: %d: provided port is already allocated", cluster.Spec.Service.WritePort),
					Field:   field.Child("writePort").String(),
				})
			}
		}
	}

	return causes
}
