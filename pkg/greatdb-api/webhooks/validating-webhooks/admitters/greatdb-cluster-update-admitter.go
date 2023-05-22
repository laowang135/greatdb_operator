package admitters

import (
	"context"
	"fmt"
	"reflect"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/utils/log"

	admissionv1 "k8s.io/api/admission/v1"
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
		causes = append(causes, ValidatingUpdateMetadata(k8sfield.NewPath("metadata"), newCluster.Labels)...)
		return utils.NewAdmissionResponse(causes)
	}
	return utils.NewPassingAdmissionResponse()
}

func ValidatingGreatDBClusterUpdateSpec(field *k8sfield.Path, cluster, oldCluster *v1alpha1.GreatDBPaxos, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause
	causes = append(causes, ValidatingUpdateSecretName(field.Child("secretName"), cluster, oldCluster, kubeClient)...)
	causes = append(causes, ValidatingPriorityClassName(field.Child("priorityClassName"), cluster.Spec.PriorityClassName, kubeClient)...)
	causes = append(causes, ValidatingImagePullSecrets(field.Child("imagePullSecrets"), cluster.Namespace, cluster.Spec.ImagePullSecrets, kubeClient)...)

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

func ValidatingUpdateMetadata(field *k8sfield.Path, labels map[string]string) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if labels == nil {
		labels = make(map[string]string)
	}

	v, ok := labels[resources.AppKubeNameLabelKey]
	if !ok {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Label %s must exist", resources.AppKubeNameLabelKey),
			Field:   field.String(),
		})
		return causes
	}
	if v != resources.AppKubeNameLabelValue {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Label %s must equal %s", resources.AppKubeNameLabelKey, resources.AppKubeNameLabelValue),
			Field:   field.String(),
		})
		return causes
	}

	v, ok = labels[resources.AppkubeManagedByLabelKey]
	if !ok {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Label %s must exist", resources.AppkubeManagedByLabelKey),
			Field:   field.String(),
		})
		return causes
	}

	if v != resources.AppkubeManagedByLabelValue {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Label %s must equal %s", resources.AppkubeManagedByLabelKey, resources.AppkubeManagedByLabelValue),
			Field:   field.String(),
		})
		return causes
	}
	return causes

}
